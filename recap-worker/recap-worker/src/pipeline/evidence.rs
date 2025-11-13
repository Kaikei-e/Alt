/// ジャンル別証拠コーパス構築モジュール。
///
/// GenreStageの出力から、各ジャンルごとに記事をグループ化し、
/// Subworkerに送信するための証拠コーパスを構築します。
use std::collections::HashMap;

use serde::{Deserialize, Serialize};
use tracing::{debug, info};
use uuid::Uuid;

use super::genre::{GenreAssignment, GenreBundle};

/// Subworkerが受け付ける文の最小文字数（空白除外）。
const MIN_SENTENCE_LENGTH_CHARS: usize = 20;

/// ジャンル別の証拠コーパス。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct EvidenceCorpus {
    pub(crate) genre: String,
    pub(crate) articles: Vec<EvidenceArticle>,
    pub(crate) total_sentences: usize,
    pub(crate) metadata: CorpusMetadata,
}

/// 証拠コーパス内の記事情報。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct EvidenceArticle {
    pub(crate) article_id: String,
    pub(crate) title: Option<String>,
    pub(crate) sentences: Vec<String>,
    pub(crate) language: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) genre_scores: Option<HashMap<String, usize>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) confidence: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) signals: Option<ArticleFeatureSignal>,
}

/// コーパスのメタデータ。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CorpusMetadata {
    pub(crate) article_count: usize,
    pub(crate) sentence_count: usize,
    pub(crate) primary_language: String,
    pub(crate) language_distribution: HashMap<String, usize>,
    pub(crate) character_count: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) classifier: Option<ClassifierStats>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub(crate) struct ArticleFeatureSignal {
    pub(crate) tfidf_sum: f32,
    pub(crate) bm25_peak: f32,
    pub(crate) token_count: usize,
    pub(crate) keyword_hits: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub(crate) struct ClassifierStats {
    pub(crate) avg_confidence: f32,
    pub(crate) max_confidence: f32,
    pub(crate) min_confidence: f32,
    pub(crate) coverage_ratio: f32,
}

/// ジャンル別にグループ化された証拠コーパスのコレクション。
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct EvidenceBundle {
    job_id: Uuid,
    pub(crate) corpora: HashMap<String, EvidenceCorpus>,
}

impl EvidenceBundle {
    /// GenreBundleから証拠コーパスを構築する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `bundle` - ジャンル付き記事バンドル
    ///
    /// # Returns
    /// ジャンル別にグループ化された証拠コーパス
    pub(crate) fn from_genre_bundle(job_id: Uuid, bundle: GenreBundle) -> Self {
        info!(
            job_id = %job_id,
            total_assignments = bundle.assignments.len(),
            "building evidence corpora from genre assignments"
        );

        // ジャンルごとに記事をグループ化
        let mut genre_groups: HashMap<String, Vec<&GenreAssignment>> = HashMap::new();

        for assignment in &bundle.assignments {
            // 各記事は複数のジャンルに属する可能性がある
            for genre in &assignment.genres {
                genre_groups
                    .entry(genre.clone())
                    .or_insert_with(Vec::new)
                    .push(assignment);
            }
        }

        // 各ジャンルグループから証拠コーパスを構築
        let mut corpora = HashMap::new();

        for (genre, assignments) in genre_groups {
            let corpus = build_corpus_for_genre(&genre, &assignments);

            debug!(
                genre = %genre,
                article_count = corpus.articles.len(),
                sentence_count = corpus.total_sentences,
                character_count = corpus.metadata.character_count,
                "built evidence corpus for genre"
            );

            corpora.insert(genre.clone(), corpus);
        }

        let evidence_bundle = Self { job_id, corpora };

        info!(
            job_id = %job_id,
            genre_count = evidence_bundle.genres().len(),
            total_articles = evidence_bundle.total_articles(),
            total_sentences = evidence_bundle.total_sentences(),
            total_characters = evidence_bundle.total_characters(),
            "completed evidence corpus construction"
        );

        evidence_bundle
    }

    /// 特定のジャンルのコーパスを取得する。
    pub(crate) fn get_corpus(&self, genre: &str) -> Option<&EvidenceCorpus> {
        self.corpora.get(genre)
    }

    /// すべてのジャンルのリストを取得する。
    pub(crate) fn genres(&self) -> Vec<String> {
        self.corpora.keys().cloned().collect()
    }

    /// コーパスの総記事数を取得する。
    pub(crate) fn total_articles(&self) -> usize {
        self.corpora.values().map(|c| c.articles.len()).sum()
    }

    /// コーパスの総文数を取得する。
    pub(crate) fn total_sentences(&self) -> usize {
        self.corpora.values().map(|c| c.total_sentences).sum()
    }

    /// コーパスの総文字数を取得する。
    pub(crate) fn total_characters(&self) -> usize {
        self.corpora
            .values()
            .map(|c| c.metadata.character_count)
            .sum()
    }
}

/// 特定のジャンルに対する証拠コーパスを構築する。
fn build_corpus_for_genre(genre: &str, assignments: &[&GenreAssignment]) -> EvidenceCorpus {
    let mut articles = Vec::new();
    let mut total_sentences = 0;
    let mut total_characters = 0;
    let mut language_counts: HashMap<String, usize> = HashMap::new();
    let mut dropped_articles = 0usize;
    let mut dropped_sentences = 0usize;
    let mut confidences: Vec<f32> = Vec::new();
    let mut supporting_articles = 0usize;

    for assignment in assignments {
        let article = &assignment.article;
        let filtered_sentences: Vec<String> = article
            .sentences
            .iter()
            .filter(|sentence| sentence_has_required_length(sentence))
            .cloned()
            .collect();

        let original_count = article.sentences.len();
        if filtered_sentences.is_empty() {
            dropped_articles += 1;
            dropped_sentences += original_count;
            debug!(
                article_id = %article.id,
                genre = %genre,
                original_sentences = original_count,
                "skipped article: all sentences shorter than minimum length"
            );
            continue;
        }

        let removed_here = original_count.saturating_sub(filtered_sentences.len());
        if removed_here > 0 {
            dropped_sentences += removed_here;
        }
        total_sentences += filtered_sentences.len();
        total_characters += filtered_sentences
            .iter()
            .map(|sentence| sentence.chars().count())
            .sum::<usize>();

        *language_counts.entry(article.language.clone()).or_insert(0) += 1;

        let keyword_hits = assignment.genre_scores.get(genre).copied().unwrap_or(0);
        if keyword_hits > 0 {
            supporting_articles += 1;
        }

        let confidence = assignment.genre_confidence.get(genre).copied();
        if let Some(conf) = confidence {
            confidences.push(conf.clamp(0.0, 1.0));
        }

        articles.push(EvidenceArticle {
            article_id: article.id.clone(),
            title: article.title.clone(),
            sentences: filtered_sentences,
            language: article.language.clone(),
            genre_scores: Some(assignment.genre_scores.clone()),
            confidence,
            signals: Some(ArticleFeatureSignal {
                tfidf_sum: assignment.feature_profile.tfidf_sum,
                bm25_peak: assignment.feature_profile.bm25_peak,
                token_count: assignment.feature_profile.token_count,
                keyword_hits,
            }),
        });
    }

    if dropped_articles > 0 || dropped_sentences > 0 {
        debug!(
            genre = %genre,
            dropped_articles,
            dropped_sentences,
            "filtered short sentences before subworker dispatch"
        );
    }

    // 最も多い言語を特定
    let primary_language = language_counts
        .iter()
        .max_by_key(|(_, count)| *count)
        .map(|(lang, _)| lang.clone())
        .unwrap_or_else(|| "und".to_string());

    let classifier = if !confidences.is_empty() {
        let sum = confidences.iter().sum::<f32>();
        let avg = sum / confidences.len() as f32;
        let max = confidences.iter().copied().fold(0.0, f32::max);
        let min = confidences.iter().copied().fold(1.0, f32::min);
        let coverage_ratio = if !articles.is_empty() {
            supporting_articles as f32 / articles.len() as f32
        } else {
            0.0
        };
        Some(ClassifierStats {
            avg_confidence: avg,
            max_confidence: max,
            min_confidence: min,
            coverage_ratio,
        })
    } else {
        None
    };

    let metadata = CorpusMetadata {
        article_count: articles.len(),
        sentence_count: total_sentences,
        primary_language,
        language_distribution: language_counts,
        character_count: total_characters,
        classifier,
    };

    EvidenceCorpus {
        genre: genre.to_string(),
        articles,
        total_sentences,
        metadata,
    }
}

fn sentence_has_required_length(sentence: &str) -> bool {
    let non_whitespace_chars = sentence.chars().filter(|c| !c.is_whitespace()).count();
    non_whitespace_chars >= MIN_SENTENCE_LENGTH_CHARS
}

#[cfg(test)]
mod tests {
    use super::super::dedup::DeduplicatedArticle;
    use super::super::genre::{FeatureProfile, GenreCandidate};
    use super::*;

    fn create_assignment(
        id: &str,
        genres: Vec<&str>,
        sentences: Vec<&str>,
        language: &str,
    ) -> GenreAssignment {
        let genre_strings: Vec<String> = genres.iter().map(|g| g.to_string()).collect();
        let token_count = sentences.len();
        let article = DeduplicatedArticle {
            id: id.to_string(),
            title: Some(format!("Title {}", id)),
            sentences: sentences.into_iter().map(String::from).collect(),
            sentence_hashes: vec![],
            language: language.to_string(),
            tags: Vec::new(),
        };

        let genre_scores = genre_strings
            .iter()
            .enumerate()
            .map(|(i, g)| (g.to_string(), 10 - i))
            .collect();
        let genre_confidence = genre_strings.iter().map(|g| (g.to_string(), 0.8)).collect();
        let feature_profile = FeatureProfile {
            tfidf_sum: 1.0,
            bm25_peak: 0.9,
            token_count,
            tag_overlap_count: 0,
        };
        let candidates = genre_strings
            .iter()
            .map(|g| GenreCandidate {
                name: g.clone(),
                score: 0.8,
                keyword_support: 8,
                classifier_confidence: 0.75,
            })
            .collect();

        GenreAssignment {
            genres: genre_strings,
            candidates,
            genre_scores,
            genre_confidence,
            feature_profile,
            article,
        }
    }

    #[test]
    fn evidence_bundle_groups_by_genre() {
        let job_id = Uuid::new_v4();
        let assignments = vec![
            create_assignment(
                "art-1",
                vec!["ai", "tech"],
                vec!["This sentence is sufficiently descriptive for clustering validation."],
                "en",
            ),
            create_assignment(
                "art-2",
                vec!["tech"],
                vec![
                    "Another sentence that easily clears the minimum character threshold.",
                    "Yet another sentence that meets the subworker requirements for processing.",
                ],
                "en",
            ),
            create_assignment(
                "art-3",
                vec!["ai"],
                vec!["これは要件を満たす十分に長い日本語の文です。"],
                "ja",
            ),
        ];

        let bundle = GenreBundle {
            job_id,
            assignments,
            genre_distribution: HashMap::new(),
        };

        let evidence = EvidenceBundle::from_genre_bundle(job_id, bundle);

        // aiとtechの2つのジャンルがある
        assert_eq!(evidence.corpora.len(), 2);
        assert!(evidence.corpora.contains_key("ai"));
        assert!(evidence.corpora.contains_key("tech"));

        // aiジャンルには2つの記事（art-1とart-3）
        let ai_corpus = evidence.get_corpus("ai").unwrap();
        assert_eq!(ai_corpus.articles.len(), 2);
        assert_eq!(ai_corpus.total_sentences, 2);
        let expected_ai_chars: usize = ai_corpus
            .articles
            .iter()
            .map(|article| {
                article
                    .sentences
                    .iter()
                    .map(|sentence| sentence.chars().count())
                    .sum::<usize>()
            })
            .sum();
        assert_eq!(ai_corpus.metadata.character_count, expected_ai_chars);

        // techジャンルには2つの記事（art-1とart-2）
        let tech_corpus = evidence.get_corpus("tech").unwrap();
        assert_eq!(tech_corpus.articles.len(), 2);
        assert_eq!(tech_corpus.total_sentences, 3);
        let expected_tech_chars: usize = tech_corpus
            .articles
            .iter()
            .map(|article| {
                article
                    .sentences
                    .iter()
                    .map(|sentence| sentence.chars().count())
                    .sum::<usize>()
            })
            .sum();
        assert_eq!(tech_corpus.metadata.character_count, expected_tech_chars);
    }

    #[test]
    fn corpus_metadata_tracks_languages() {
        let job_id = Uuid::new_v4();
        let assignments = vec![
            create_assignment(
                "art-1",
                vec!["ai"],
                vec!["This sentence comfortably exceeds the minimum character count required."],
                "en",
            ),
            create_assignment(
                "art-2",
                vec!["ai"],
                vec![
                    "Another sentence that easily passes the minimum length check for the worker.",
                ],
                "en",
            ),
            create_assignment(
                "art-3",
                vec!["ai"],
                vec!["こちらも条件を満たす十分な長さの日本語の文です。"],
                "ja",
            ),
        ];

        let bundle = GenreBundle {
            job_id,
            assignments,
            genre_distribution: HashMap::new(),
        };

        let evidence = EvidenceBundle::from_genre_bundle(job_id, bundle);
        let ai_corpus = evidence.get_corpus("ai").unwrap();

        assert_eq!(ai_corpus.metadata.article_count, 3);
        assert_eq!(ai_corpus.metadata.sentence_count, 3);
        assert_eq!(ai_corpus.metadata.primary_language, "en");
        assert_eq!(ai_corpus.metadata.language_distribution.get("en"), Some(&2));
        assert_eq!(ai_corpus.metadata.language_distribution.get("ja"), Some(&1));
        let expected_characters: usize = ai_corpus
            .articles
            .iter()
            .map(|article| {
                article
                    .sentences
                    .iter()
                    .map(|sentence| sentence.chars().count())
                    .sum::<usize>()
            })
            .sum();
        assert_eq!(ai_corpus.metadata.character_count, expected_characters);
    }

    #[test]
    fn evidence_bundle_utility_methods() {
        let job_id = Uuid::new_v4();
        let assignments = vec![
            create_assignment(
                "art-1",
                vec!["ai"],
                vec![
                    "First sentence that meets the threshold with comfortable margin for safety.",
                    "Second sentence that is also long enough to stay compliant with validation rules.",
                ],
                "en",
            ),
            create_assignment(
                "art-2",
                vec!["tech"],
                vec!["Third sentence that is sufficiently verbose for the new validation checks."],
                "en",
            ),
        ];

        let bundle = GenreBundle {
            job_id,
            assignments,
            genre_distribution: HashMap::new(),
        };

        let evidence = EvidenceBundle::from_genre_bundle(job_id, bundle);

        assert_eq!(evidence.genres().len(), 2);
        assert_eq!(evidence.total_articles(), 2);
        assert_eq!(evidence.total_sentences(), 3);
        let expected_characters: usize = evidence
            .corpora
            .values()
            .flat_map(|corpus| corpus.articles.iter())
            .map(|article| {
                article
                    .sentences
                    .iter()
                    .map(|sentence| sentence.chars().count())
                    .sum::<usize>()
            })
            .sum();
        assert_eq!(evidence.total_characters(), expected_characters);
    }

    #[test]
    fn evidence_article_includes_genre_scores() {
        let job_id = Uuid::new_v4();
        let assignments = vec![create_assignment(
            "art-1",
            vec!["ai", "tech"],
            vec!["This sentence is long enough to survive filtering before subworker dispatch."],
            "en",
        )];

        let bundle = GenreBundle {
            job_id,
            assignments,
            genre_distribution: HashMap::new(),
        };

        let evidence = EvidenceBundle::from_genre_bundle(job_id, bundle);
        let ai_corpus = evidence.get_corpus("ai").unwrap();
        let article = &ai_corpus.articles[0];

        assert!(article.genre_scores.is_some());
        let scores = article.genre_scores.as_ref().unwrap();
        assert!(scores.contains_key("ai"));
        assert!(scores.contains_key("tech"));
    }

    #[test]
    fn short_sentences_are_filtered_out() {
        let job_id = Uuid::new_v4();
        let assignments = vec![create_assignment(
            "art-1",
            vec!["ai"],
            vec![
                "Apple Inc.",
                "Short.",
                "これは短い。",
                "This sentence clearly satisfies the minimum length requirement imposed by the subworker schema.",
            ],
            "en",
        )];

        let bundle = GenreBundle {
            job_id,
            assignments,
            genre_distribution: HashMap::new(),
        };

        let evidence = EvidenceBundle::from_genre_bundle(job_id, bundle);
        let ai_corpus = evidence.get_corpus("ai").unwrap();

        assert_eq!(ai_corpus.articles.len(), 1);
        assert_eq!(ai_corpus.articles[0].sentences.len(), 1);
        assert_eq!(
            ai_corpus.articles[0].sentences[0],
            "This sentence clearly satisfies the minimum length requirement imposed by the subworker schema."
        );
        assert_eq!(ai_corpus.total_sentences, 1);
    }
}
