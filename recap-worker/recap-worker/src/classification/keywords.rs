//! 高速なジャンルキーワード照合データ構造。
use aho_corasick::{AhoCorasick, AhoCorasickBuilder, MatchKind};
use fst::{automaton::Str, IntoStreamer, Map, MapBuilder};
use once_cell::sync::Lazy;
use std::borrow::Cow;
use std::collections::HashMap;
use std::io;
use std::iter::FromIterator;

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
    map: Map<Vec<u8>>,
}

impl KeywordMatcher {
    pub fn new(entries: &[KeywordEntry]) -> io::Result<Self> {
        let patterns: Vec<&str> = entries.iter().map(|entry| entry.phrase.as_str()).collect();
        let ac = AhoCorasickBuilder::new()
            .match_kind(MatchKind::LeftmostLongest)
            .ascii_case_insensitive(true)
            .build(&patterns)?;

        let mut kv_pairs: Vec<(Vec<u8>, u64)> = entries
            .iter()
            .enumerate()
            .map(|(idx, entry)| (entry.phrase.to_lowercase().into_bytes(), idx as u64))
            .collect();
        kv_pairs.sort_unstable();

        let mut buffer = Vec::new();
        {
            let mut builder = MapBuilder::new(&mut buffer)?;
            for (key, value) in kv_pairs {
                builder.insert(&key, value)?;
            }
            builder.finish()?;
        }

        let map = Map::new(buffer)?;

        Ok(Self { ac, map })
    }

    #[must_use]
    pub fn find_matches<'a>(&self, text: &'a str) -> Vec<Match<'a>> {
        let mut results = Vec::new();
        for mat in self.ac.find_iter(text) {
            if let Some(idx) = self.lookup_index(&text[mat.start()..mat.end()]) {
                results.push(Match {
                    index: idx,
                    span: mat,
                });
            }
        }
        results
    }

    fn lookup_index(&self, phrase: &str) -> Option<u64> {
        let lower = phrase.to_lowercase();
        let bytes = lower.as_bytes();
        self.map
            .range()
            .ge(Str::from(bytes))
            .into_stream()
            .next()
            .and_then(|(key, value)| {
                // verify exact match
                if key == bytes {
                    Some(value)
                } else {
                    None
                }
            })
    }
}

/// 一致情報。
#[derive(Debug, Clone)]
pub struct Match<'a> {
    pub index: u64,
    pub span: aho_corasick::Match,
}

/// コンパイル済みのデフォルト辞書。
pub static DEFAULT_KEYWORDS: Lazy<Vec<KeywordEntry>> = Lazy::new(|| {
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
pub fn accumulate_scores(entries: &[KeywordEntry], matches: &[Match<'_>]) -> HashMap<String, u32> {
    let mut scores: HashMap<String, u32> = HashMap::new();
    for m in matches {
        if let Some(entry) = entries.get(m.index as usize) {
            *scores.entry(entry.genre.clone()).or_insert(0) += entry.weight as u32;
        }
    }
    scores
}
