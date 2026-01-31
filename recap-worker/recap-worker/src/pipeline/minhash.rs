//! MinHash LSH (Locality-Sensitive Hashing) implementation for near-duplicate detection.
//!
//! This module provides efficient near-duplicate detection using MinHash signatures
//! and Locality-Sensitive Hashing. It's designed for large-scale document deduplication.
//!
//! # Algorithm Overview
//!
//! 1. **Shingling**: Convert documents to sets of n-gram shingles
//! 2. **MinHash**: Compute compact signatures using min-wise hashing
//! 3. **LSH Banding**: Group signature rows into bands for efficient candidate retrieval
//!
//! # References
//!
//! - [MinHash LSH in Milvus](https://milvus.io/blog/minhash-lsh-in-milvus-the-secret-weapon-for-fighting-duplicates-in-llm-training-data.md)
//! - [Rensa: High-Performance MinHash in Rust](https://github.com/beowolx/rensa)

use rustc_hash::{FxHashMap, FxHashSet};
use smallvec::SmallVec;
use xxhash_rust::xxh3::xxh3_64_with_seed;

/// Default number of hash permutations for MinHash signature.
const DEFAULT_NUM_PERM: usize = 128;

/// Default shingle size (character n-grams).
const DEFAULT_SHINGLE_SIZE: usize = 5;

/// Maximum number of candidates to return from LSH query.
const MAX_CANDIDATES: usize = 100;

/// MinHash signature for a document.
#[derive(Debug, Clone)]
pub struct MinHashSignature {
    /// Document identifier.
    pub doc_id: String,
    /// MinHash signature values.
    pub signature: Vec<u64>,
    /// Original document index (for tracking).
    pub index: usize,
}

impl MinHashSignature {
    /// Create a new MinHash signature.
    pub fn new(doc_id: String, signature: Vec<u64>, index: usize) -> Self {
        Self {
            doc_id,
            signature,
            index,
        }
    }

    /// Compute Jaccard similarity estimate from MinHash signatures.
    pub fn similarity(&self, other: &Self) -> f64 {
        if self.signature.len() != other.signature.len() {
            return 0.0;
        }

        let matches = self
            .signature
            .iter()
            .zip(other.signature.iter())
            .filter(|(a, b)| a == b)
            .count();

        matches as f64 / self.signature.len() as f64
    }
}

/// Result of duplicate detection.
#[derive(Debug, Clone)]
pub struct DuplicateResult {
    /// Document ID being checked.
    pub doc_id: String,
    /// Whether the document is a duplicate.
    pub is_duplicate: bool,
    /// Document ID of the original (if duplicate).
    pub original_doc_id: Option<String>,
    /// Estimated Jaccard similarity (if duplicate).
    pub similarity: Option<f64>,
}

/// MinHash LSH index for efficient near-duplicate detection.
///
/// Uses banding technique to achieve sub-linear query time while
/// maintaining high probability of detecting similar documents.
#[derive(Debug)]
pub struct MinHashLSH {
    /// Number of hash permutations.
    num_perm: usize,
    /// Number of bands for LSH.
    num_bands: usize,
    /// Number of rows per band.
    rows_per_band: usize,
    /// LSH buckets: band_index -> (bucket_hash -> doc_indices).
    buckets: Vec<FxHashMap<u64, SmallVec<[usize; 4]>>>,
    /// Stored signatures for similarity verification.
    signatures: Vec<MinHashSignature>,
    /// Shingle size for text processing.
    shingle_size: usize,
    /// Seeds for hash permutations.
    seeds: Vec<u64>,
}

impl MinHashLSH {
    /// Create a new MinHash LSH index with default parameters.
    ///
    /// Uses 128 permutations with automatic band calculation for 0.5 threshold.
    pub fn new() -> Self {
        Self::with_params(DEFAULT_NUM_PERM, 0.5, DEFAULT_SHINGLE_SIZE)
    }

    /// Create a new MinHash LSH index with custom threshold.
    ///
    /// # Arguments
    ///
    /// * `threshold` - Jaccard similarity threshold (0.0 - 1.0).
    pub fn with_threshold(threshold: f64) -> Self {
        Self::with_params(DEFAULT_NUM_PERM, threshold, DEFAULT_SHINGLE_SIZE)
    }

    /// Create a new MinHash LSH index with full parameters.
    ///
    /// # Arguments
    ///
    /// * `num_perm` - Number of hash permutations (signature size).
    /// * `threshold` - Jaccard similarity threshold for duplicate detection.
    /// * `shingle_size` - Character n-gram size for shingling.
    pub fn with_params(num_perm: usize, threshold: f64, shingle_size: usize) -> Self {
        let (num_bands, rows_per_band) = optimal_lsh_params(num_perm, threshold);

        // Generate deterministic seeds for reproducibility
        let seeds: Vec<u64> = (0..num_perm as u64)
            .map(|i| i.wrapping_mul(0x517c_c1b7_2722_0a95))
            .collect();

        let buckets = (0..num_bands).map(|_| FxHashMap::default()).collect();

        Self {
            num_perm,
            num_bands,
            rows_per_band,
            buckets,
            signatures: Vec::new(),
            shingle_size,
            seeds,
        }
    }

    /// Compute MinHash signature for a document.
    ///
    /// # Arguments
    ///
    /// * `doc_id` - Document identifier.
    /// * `content` - Document text content.
    /// * `index` - Document index for tracking.
    pub fn compute_signature(&self, doc_id: &str, content: &str, index: usize) -> MinHashSignature {
        let shingles = self.create_shingles(content);
        let signature = self.minhash(&shingles);

        MinHashSignature::new(doc_id.to_string(), signature, index)
    }

    /// Create character n-gram shingles from text.
    fn create_shingles(&self, text: &str) -> FxHashSet<u64> {
        let chars: Vec<char> = text.chars().collect();
        let mut shingles = FxHashSet::default();

        if chars.len() < self.shingle_size {
            // For very short texts, use the whole text as a single shingle
            let hash = xxh3_64_with_seed(text.as_bytes(), 0);
            shingles.insert(hash);
            return shingles;
        }

        for window in chars.windows(self.shingle_size) {
            let shingle: String = window.iter().collect();
            let hash = xxh3_64_with_seed(shingle.as_bytes(), 0);
            shingles.insert(hash);
        }

        shingles
    }

    /// Compute MinHash signature for a set of shingles.
    fn minhash(&self, shingles: &FxHashSet<u64>) -> Vec<u64> {
        if shingles.is_empty() {
            return vec![u64::MAX; self.num_perm];
        }

        let mut signature = vec![u64::MAX; self.num_perm];

        for &shingle in shingles {
            for (i, &seed) in self.seeds.iter().enumerate() {
                let hash = xxh3_64_with_seed(&shingle.to_le_bytes(), seed);
                signature[i] = signature[i].min(hash);
            }
        }

        signature
    }

    /// Insert a signature into the LSH index.
    ///
    /// # Arguments
    ///
    /// * `signature` - MinHash signature to insert.
    pub fn insert(&mut self, signature: MinHashSignature) {
        let sig_idx = self.signatures.len();

        // Insert into each band's bucket
        for (band_idx, chunk) in signature.signature.chunks(self.rows_per_band).enumerate() {
            if band_idx >= self.num_bands {
                break;
            }

            let bucket_hash = hash_band(chunk);
            self.buckets[band_idx]
                .entry(bucket_hash)
                .or_default()
                .push(sig_idx);
        }

        self.signatures.push(signature);
    }

    /// Query for candidate duplicates of a document.
    ///
    /// Returns indices of candidate duplicates that share at least one LSH bucket.
    ///
    /// # Arguments
    ///
    /// * `signature` - MinHash signature to query.
    pub fn query(&self, signature: &MinHashSignature) -> FxHashSet<usize> {
        let mut candidates = FxHashSet::default();

        for (band_idx, chunk) in signature.signature.chunks(self.rows_per_band).enumerate() {
            if band_idx >= self.num_bands {
                break;
            }

            let bucket_hash = hash_band(chunk);
            if let Some(indices) = self.buckets[band_idx].get(&bucket_hash) {
                for &idx in indices {
                    if candidates.len() >= MAX_CANDIDATES {
                        return candidates;
                    }
                    candidates.insert(idx);
                }
            }
        }

        candidates
    }

    /// Check if a document is a duplicate and optionally insert it.
    ///
    /// # Arguments
    ///
    /// * `doc_id` - Document identifier.
    /// * `content` - Document text content.
    /// * `index` - Document index.
    /// * `threshold` - Similarity threshold for duplicate detection.
    /// * `insert_if_unique` - Whether to insert the signature if not a duplicate.
    pub fn check_duplicate(
        &mut self,
        doc_id: &str,
        content: &str,
        index: usize,
        threshold: f64,
        insert_if_unique: bool,
    ) -> DuplicateResult {
        let signature = self.compute_signature(doc_id, content, index);

        // Query for candidates
        let candidates = self.query(&signature);

        // Verify candidates with actual similarity computation
        for &candidate_idx in &candidates {
            if let Some(candidate_sig) = self.signatures.get(candidate_idx) {
                let similarity = signature.similarity(candidate_sig);
                if similarity >= threshold {
                    return DuplicateResult {
                        doc_id: doc_id.to_string(),
                        is_duplicate: true,
                        original_doc_id: Some(candidate_sig.doc_id.clone()),
                        similarity: Some(similarity),
                    };
                }
            }
        }

        // Not a duplicate, optionally insert
        if insert_if_unique {
            self.insert(signature);
        }

        DuplicateResult {
            doc_id: doc_id.to_string(),
            is_duplicate: false,
            original_doc_id: None,
            similarity: None,
        }
    }

    /// Get the number of indexed documents.
    pub fn len(&self) -> usize {
        self.signatures.len()
    }

    /// Check if the index is empty.
    pub fn is_empty(&self) -> bool {
        self.signatures.is_empty()
    }

    /// Clear all indexed documents.
    pub fn clear(&mut self) {
        self.signatures.clear();
        for bucket in &mut self.buckets {
            bucket.clear();
        }
    }

    /// Get LSH parameters.
    pub fn params(&self) -> (usize, usize, usize) {
        (self.num_perm, self.num_bands, self.rows_per_band)
    }
}

impl Default for MinHashLSH {
    fn default() -> Self {
        Self::new()
    }
}

/// Hash a band (chunk of signature) into a single bucket key.
fn hash_band(band: &[u64]) -> u64 {
    let mut result = 0u64;
    for (i, &value) in band.iter().enumerate() {
        result ^= value.rotate_left((i * 7) as u32);
    }
    result
}

/// Calculate optimal LSH parameters for a given threshold.
///
/// Uses the formula: P(candidate) ≈ 1 - (1 - s^r)^b
/// where s = similarity, r = rows per band, b = number of bands.
///
/// # Arguments
///
/// * `num_perm` - Total number of hash permutations.
/// * `threshold` - Target similarity threshold.
///
/// # Returns
///
/// (num_bands, rows_per_band)
fn optimal_lsh_params(num_perm: usize, threshold: f64) -> (usize, usize) {
    let threshold = threshold.clamp(0.1, 0.99);

    // Try different band configurations
    let mut best_bands = 1;
    let mut best_rows = num_perm;
    let mut best_score = f64::MAX;

    for b in 1..=num_perm {
        if !num_perm.is_multiple_of(b) {
            continue;
        }

        let r = num_perm / b;

        // Probability at threshold
        let p_at_threshold = 1.0 - (1.0 - threshold.powi(r as i32)).powi(b as i32);

        // We want p_at_threshold to be close to 0.5 (inflection point)
        let score = (p_at_threshold - 0.5).abs();

        if score < best_score {
            best_score = score;
            best_bands = b;
            best_rows = r;
        }
    }

    (best_bands, best_rows)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_minhash_signature_similarity() {
        let lsh = MinHashLSH::new();

        let sig1 = lsh.compute_signature("doc1", "The quick brown fox jumps over the lazy dog", 0);
        let sig2 = lsh.compute_signature("doc2", "The quick brown fox jumps over the lazy cat", 1);
        let sig3 =
            lsh.compute_signature("doc3", "Completely different text about something else", 2);

        // Similar documents should have high similarity
        let sim_1_2 = sig1.similarity(&sig2);
        assert!(
            sim_1_2 > 0.5,
            "Similar docs should have similarity > 0.5, got {}",
            sim_1_2
        );

        // Different documents should have low similarity
        let sim_1_3 = sig1.similarity(&sig3);
        assert!(
            sim_1_3 < 0.3,
            "Different docs should have similarity < 0.3, got {}",
            sim_1_3
        );
    }

    #[test]
    fn test_minhash_lsh_duplicate_detection() {
        let mut lsh = MinHashLSH::with_threshold(0.5);

        // Insert first document
        let result1 = lsh.check_duplicate(
            "doc1",
            "The quick brown fox jumps over the lazy dog",
            0,
            0.5,
            true,
        );
        assert!(!result1.is_duplicate);

        // Check similar document
        let _result2 = lsh.check_duplicate(
            "doc2",
            "The quick brown fox jumps over the lazy cat",
            1,
            0.5,
            true,
        );
        // May or may not be detected as duplicate depending on threshold

        // Check completely different document
        let result3 = lsh.check_duplicate(
            "doc3",
            "Completely different text about something else entirely",
            2,
            0.5,
            true,
        );
        assert!(!result3.is_duplicate);

        // Check exact duplicate
        let result4 = lsh.check_duplicate(
            "doc4",
            "The quick brown fox jumps over the lazy dog",
            3,
            0.5,
            false,
        );
        assert!(result4.is_duplicate);
        assert_eq!(result4.original_doc_id, Some("doc1".to_string()));
    }

    #[test]
    fn test_minhash_lsh_params() {
        let lsh = MinHashLSH::with_params(128, 0.5, 5);
        let (num_perm, num_bands, rows_per_band) = lsh.params();

        assert_eq!(num_perm, 128);
        assert_eq!(num_bands * rows_per_band, 128);
    }

    #[test]
    fn test_optimal_lsh_params() {
        // For threshold 0.5 with 128 permutations
        let (bands, rows) = optimal_lsh_params(128, 0.5);
        assert_eq!(bands * rows, 128);
        assert!(bands > 1);
        assert!(rows > 1);
    }

    #[test]
    fn test_empty_and_short_text() {
        let lsh = MinHashLSH::new();

        // Empty text
        let sig_empty = lsh.compute_signature("empty", "", 0);
        assert_eq!(sig_empty.signature.len(), DEFAULT_NUM_PERM);

        // Very short text
        let sig_short = lsh.compute_signature("short", "Hi", 1);
        assert_eq!(sig_short.signature.len(), DEFAULT_NUM_PERM);
    }

    #[test]
    fn test_japanese_text() {
        let mut lsh = MinHashLSH::with_threshold(0.5);

        let result1 = lsh.check_duplicate(
            "ja1",
            "人工知能の技術は近年急速に発展を遂げている。機械学習やディープラーニングの進歩により、様々な分野で活用されている。",
            0,
            0.5,
            true,
        );
        assert!(!result1.is_duplicate);

        // Similar Japanese text
        let _result2 = lsh.check_duplicate(
            "ja2",
            "人工知能の技術は近年急速に発展を遂げている。機械学習やディープラーニングの進歩により、多くの分野で活用されている。",
            1,
            0.5,
            true,
        );
        // Should detect high similarity

        // Different Japanese text
        let result3 = lsh.check_duplicate(
            "ja3",
            "今日の天気は晴れです。明日は雨が降るかもしれません。",
            2,
            0.5,
            true,
        );
        assert!(!result3.is_duplicate);
    }

    #[test]
    fn test_lsh_clear() {
        let mut lsh = MinHashLSH::new();

        lsh.check_duplicate("doc1", "Some text content", 0, 0.5, true);
        lsh.check_duplicate("doc2", "Other text content", 1, 0.5, true);

        assert_eq!(lsh.len(), 2);

        lsh.clear();

        assert!(lsh.is_empty());
        assert_eq!(lsh.len(), 0);
    }
}
