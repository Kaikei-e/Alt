use crate::parser::EnrichedLogEntry;
use std::collections::VecDeque;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tokio::sync::Notify;
use tokio::time::{sleep_until, Instant as TokioInstant};
use uuid::Uuid;
use serde::{Serialize, Deserialize};

#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub enum BatchType {
    SizeBased,
    TimeBased,
    MemoryBased,
}

#[derive(Debug, Clone)]
pub struct BatchConfig {
    pub max_size: usize,
    pub max_wait_time: Duration,
    pub max_memory_size: usize, // in bytes
}

impl Default for BatchConfig {
    fn default() -> Self {
        Self {
            max_size: 10_000,
            max_wait_time: Duration::from_millis(500),
            max_memory_size: 10 * 1024 * 1024, // 10MB
        }
    }
}

#[derive(Debug, Clone)]
pub struct Batch {
    id: String,
    entries: Vec<EnrichedLogEntry>,
    batch_type: BatchType,
    created_at: Instant,
    estimated_size: usize,
}

impl Batch {
    pub fn new(entries: Vec<EnrichedLogEntry>, batch_type: BatchType) -> Self {
        let estimated_size = entries.iter()
            .map(|entry| estimate_entry_size(entry))
            .sum();
            
        Self {
            id: Uuid::new_v4().to_string(),
            entries,
            batch_type,
            created_at: Instant::now(),
            estimated_size,
        }
    }
    
    pub fn with_id(id: String, entries: Vec<EnrichedLogEntry>, batch_type: BatchType, estimated_size: usize) -> Self {
        Self {
            id,
            entries,
            batch_type,
            created_at: Instant::now(),
            estimated_size,
        }
    }
    
    pub fn id(&self) -> &str {
        &self.id
    }
    
    pub fn size(&self) -> usize {
        self.entries.len()
    }
    
    pub fn entries(&self) -> &[EnrichedLogEntry] {
        &self.entries
    }
    
    pub fn into_entries(self) -> Vec<EnrichedLogEntry> {
        self.entries
    }
    
    pub fn batch_type(&self) -> BatchType {
        self.batch_type
    }
    
    pub fn created_at(&self) -> Instant {
        self.created_at
    }
    
    pub fn estimated_memory_size(&self) -> usize {
        self.estimated_size
    }
    
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

#[derive(Clone)]
pub struct BatchFormer {
    inner: Arc<Mutex<BatchFormerInner>>,
    config: BatchConfig,
    notify: Arc<Notify>,
}

struct BatchFormerInner {
    pending_entries: VecDeque<EnrichedLogEntry>,
    current_memory_size: usize,
    batch_start_time: Option<TokioInstant>,
    ready_batches: VecDeque<Batch>,
}

impl BatchFormer {
    pub fn new(config: BatchConfig) -> Self {
        let inner = BatchFormerInner {
            pending_entries: VecDeque::new(),
            current_memory_size: 0,
            batch_start_time: None,
            ready_batches: VecDeque::new(),
        };
        
        Self {
            inner: Arc::new(Mutex::new(inner)),
            config,
            notify: Arc::new(Notify::new()),
        }
    }
    
    pub async fn add_entry(&self, entry: EnrichedLogEntry) -> Result<(), crate::buffer::BufferError> {
        let entry_size = estimate_entry_size(&entry);
        let should_batch;
        let batch_type;
        
        {
            let mut inner = self.inner.lock().unwrap();
            
            // Set batch start time if this is the first entry
            if inner.pending_entries.is_empty() {
                inner.batch_start_time = Some(TokioInstant::now());
            }
            
            inner.pending_entries.push_back(entry);
            inner.current_memory_size += entry_size;
            
            // Check batching conditions
            should_batch = inner.pending_entries.len() >= self.config.max_size
                || inner.current_memory_size >= self.config.max_memory_size;
                
            batch_type = if inner.pending_entries.len() >= self.config.max_size {
                BatchType::SizeBased
            } else {
                BatchType::MemoryBased
            };
        }
        
        if should_batch {
            self.create_batch(batch_type).await;
        } else {
            // Start timeout timer if not already running
            self.maybe_start_timeout_timer().await;
        }
        
        Ok(())
    }
    
    pub async fn next_batch(&mut self) -> Option<Batch> {
        // Check for ready batches first
        {
            let mut inner = self.inner.lock().unwrap();
            if let Some(batch) = inner.ready_batches.pop_front() {
                return Some(batch);
            }
        }
        
        // Wait for batch to be ready
        self.notify.notified().await;
        
        let mut inner = self.inner.lock().unwrap();
        inner.ready_batches.pop_front()
    }
    
    pub fn has_ready_batch(&self) -> bool {
        let inner = self.inner.lock().unwrap();
        !inner.ready_batches.is_empty()
    }
    
    async fn create_batch(&self, batch_type: BatchType) {
        let entries: Vec<EnrichedLogEntry>;
        {
            let mut inner = self.inner.lock().unwrap();
            entries = inner.pending_entries.drain(..).collect();
            inner.current_memory_size = 0;
            inner.batch_start_time = None;
        }
        
        if !entries.is_empty() {
            let batch = Batch::new(entries, batch_type);
            
            {
                let mut inner = self.inner.lock().unwrap();
                inner.ready_batches.push_back(batch);
            }
            
            self.notify.notify_one();
        }
    }
    
    async fn maybe_start_timeout_timer(&self) {
        let should_start_timer;
        let deadline;
        
        {
            let inner = self.inner.lock().unwrap();
            should_start_timer = inner.batch_start_time.is_some() && !inner.pending_entries.is_empty();
            deadline = inner.batch_start_time.unwrap_or_else(|| TokioInstant::now()) + self.config.max_wait_time;
        }
        
        if should_start_timer {
            let former = self.clone();
            tokio::spawn(async move {
                sleep_until(deadline).await;
                former.create_batch(BatchType::TimeBased).await;
            });
        }
    }
}

fn estimate_entry_size(entry: &EnrichedLogEntry) -> usize {
    // Rough estimation of memory usage
    let base_size = std::mem::size_of::<EnrichedLogEntry>();
    let string_sizes = entry.service_type.len()
        + entry.log_type.len()
        + entry.message.len()
        + entry.timestamp.len()
        + entry.stream.len()
        + entry.container_id.len()
        + entry.service_name.len();
        
    let optional_sizes = entry.method.as_ref().map_or(0, |s| s.len())
        + entry.path.as_ref().map_or(0, |s| s.len())
        + entry.ip_address.as_ref().map_or(0, |s| s.len())
        + entry.user_agent.as_ref().map_or(0, |s| s.len())
        + entry.service_group.as_ref().map_or(0, |s| s.len());
    
    let fields_size = entry.fields.iter()
        .map(|(k, v)| k.len() + v.len())
        .sum::<usize>();
    
    base_size + string_sizes + optional_sizes + fields_size
}