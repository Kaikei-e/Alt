//! 高速なジャンルキーワード照合データ構造。
use aho_corasick::{AhoCorasick, AhoCorasickBuilder, MatchKind};
use fst::{IntoStreamer, Map, Streamer};
use std::sync::LazyLock;
use std::collections::HashMap;
use std::io;
use std::sync::Arc;

/// ジャンルごとのキーワード定義。
#[derive(Debug, Clone)]
pub struct KeywordEntry {
    pub genre: String,
    pub weight: u16,
    pub phrase: String,
}

/// マップ付きの Aho-Corasick 検索構造体。
#[derive(Debug)]
pub struct KeywordMatcher {
    ac: AhoCorasick,
    map: Arc<Map<Vec<u8>>>,
}

impl KeywordMatcher {
    pub fn new(entries: &[KeywordEntry]) -> io::Result<Self> {
        let patterns: Vec<&str> = entries.iter().map(|entry| entry.phrase.as_str()).collect();
        let ac = AhoCorasickBuilder::new()
            .match_kind(MatchKind::LeftmostLongest)
            .ascii_case_insensitive(true)
            .build(&patterns)
            .map_err(to_io)?;

        let mut kv_pairs: Vec<(Vec<u8>, u64)> = entries
            .iter()
            .enumerate()
            .map(|(idx, entry)| (entry.phrase.to_lowercase().into_bytes(), idx as u64))
            .collect();
        kv_pairs.sort_unstable();

        let map = build_map(kv_pairs)?;

        Ok(Self {
            ac,
            map: Arc::new(map),
        })
    }

    #[must_use]
    pub fn find_matches(&self, text: &str) -> Vec<Match> {
        let mut results = Vec::new();
        for mat in self.ac.find_iter(text) {
            if let Some(idx) = self.lookup_index(&text[mat.start()..mat.end()]) {
                results.push(Match { index: idx });
            }
        }
        results
    }

    fn lookup_index(&self, phrase: &str) -> Option<u64> {
        let lower = phrase.to_lowercase();
        let bytes = lower.as_bytes();
        let mut stream = self.map.range().into_stream();
        while let Some((key, value)) = stream.next() {
            if key == bytes {
                return Some(value);
            }
        }
        None
    }
}

/// 一致情報。
#[derive(Debug, Clone)]
pub struct Match {
    pub index: u64,
}

/// コンパイル済みのデフォルト辞書。
pub static DEFAULT_KEYWORDS: LazyLock<Vec<KeywordEntry>> = LazyLock::new(|| {
    vec![
        KeywordEntry {
            genre: "ai".to_string(),
            weight: 5,
            phrase: "artificial intelligence".to_string(),
        },
        KeywordEntry {
            genre: "ai".to_string(),
            weight: 5,
            phrase: "machine learning".to_string(),
        },
        KeywordEntry {
            genre: "ai".to_string(),
            weight: 4,
            phrase: "deep learning".to_string(),
        },
        KeywordEntry {
            genre: "tech".to_string(),
            weight: 3,
            phrase: "cloud computing".to_string(),
        },
        KeywordEntry {
            genre: "tech".to_string(),
            weight: 2,
            phrase: "api".to_string(),
        },
        KeywordEntry {
            genre: "business".to_string(),
            weight: 4,
            phrase: "merger".to_string(),
        },
        KeywordEntry {
            genre: "business".to_string(),
            weight: 4,
            phrase: "funding round".to_string(),
        },
        KeywordEntry {
            genre: "business".to_string(),
            weight: 3,
            phrase: "ipo".to_string(),
        },
        KeywordEntry {
            genre: "politics".to_string(),
            weight: 3,
            phrase: "election".to_string(),
        },
        KeywordEntry {
            genre: "politics".to_string(),
            weight: 3,
            phrase: "parliament".to_string(),
        },
        KeywordEntry {
            genre: "sports".to_string(),
            weight: 3,
            phrase: "tournament".to_string(),
        },
        KeywordEntry {
            genre: "sports".to_string(),
            weight: 3,
            phrase: "championship".to_string(),
        },
    ]
});

/// 既定辞書から matcher を構築する。
#[must_use]
pub fn default_matcher() -> KeywordMatcher {
    KeywordMatcher::new(&DEFAULT_KEYWORDS).expect("default keyword matcher")
}

/// マッチ結果からジャンルごとの加重スコアを計算する。
#[must_use]
pub fn accumulate_scores(entries: &[KeywordEntry], matches: &[Match]) -> HashMap<String, u32> {
    let mut scores: HashMap<String, u32> = HashMap::new();
    for m in matches {
        #[allow(clippy::cast_possible_truncation)]
        if let Some(entry) = entries.get(m.index as usize) {
            *scores.entry(entry.genre.clone()).or_insert(0) += u32::from(entry.weight);
        }
    }
    scores
}

fn build_map(pairs: Vec<(Vec<u8>, u64)>) -> io::Result<Map<Vec<u8>>> {
    Map::from_iter(pairs).map_err(to_io)
}

fn to_io<E: std::error::Error + Send + Sync + 'static>(err: E) -> io::Error {
    io::Error::other(err)
}
