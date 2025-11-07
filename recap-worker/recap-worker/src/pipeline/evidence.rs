/// ジャンル別証拠コーパス構築モジュール。
///
/// GenreStageの出力から、各ジャンルごとに記事をグループ化し、
/// Subworkerに送信するための証拠コーパスを構築します。
use std::collections::HashMap;

use serde::{Deserialize, Serialize};
use tracing::{debug, info};
use uuid::Uuid;

use super::genre::{GenreAssignment, GenreBundle};

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
}

/// コーパスのメタデータ。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CorpusMetadata {
    pub(crate) article_count: usize,
    pub(crate) sentence_count: usize,
    pub(crate) primary_language: String,
    pub(crate) language_distribution: HashMap<String, usize>,
}

/// ジャンル別にグループ化された証拠コーパスのコレクション。
#[derive(Debug, Clone)]
pub(crate) struct EvidenceBundle {
    pub(crate) job_id: Uuid,
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
                "built evidence corpus for genre"
            );

            corpora.insert(genre.clone(), corpus);
        }

        info!(
            job_id = %job_id,
            genre_count = corpora.len(),
            "completed evidence corpus construction"
        );

        Self { job_id, corpora }
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
}

/// 特定のジャンルに対する証拠コーパスを構築する。
fn build_corpus_for_genre(genre: &str, assignments: &[&GenreAssignment]) -> EvidenceCorpus {
    let mut articles = Vec::new();
    let mut total_sentences = 0;
    let mut language_counts: HashMap<String, usize> = HashMap::new();

    for assignment in assignments {
        let article = &assignment.article;
        let sentence_count = article.sentences.len();
        total_sentences += sentence_count;

        *language_counts.entry(article.language.clone()).or_insert(0) += 1;

        articles.push(EvidenceArticle {
            article_id: article.id.clone(),
            title: article.title.clone(),
            sentences: article.sentences.clone(),
            language: article.language.clone(),
            genre_scores: Some(assignment.genre_scores.clone()),
        });
    }

    // 最も多い言語を特定
    let primary_language = language_counts
        .iter()
        .max_by_key(|(_, count)| *count)
        .map(|(lang, _)| lang.clone())
        .unwrap_or_else(|| "und".to_string());

    let metadata = CorpusMetadata {
        article_count: articles.len(),
        sentence_count: total_sentences,
        primary_language,
        language_distribution: language_counts,
    };

    EvidenceCorpus {
        genre: genre.to_string(),
        articles,
        total_sentences,
        metadata,
    }
}

#[cfg(test)]
mod tests {
    use super::super::dedup::DeduplicatedArticle;
    use super::*;

    fn create_assignment(
        id: &str,
        genres: Vec<&str>,
        sentences: Vec<&str>,
        language: &str,
    ) -> GenreAssignment {
        let article = DeduplicatedArticle {
            id: id.to_string(),
            title: Some(format!("Title {}", id)),
            sentences: sentences.into_iter().map(String::from).collect(),
            sentence_hashes: vec![],
            language: language.to_string(),
        };

        let genre_scores = genres
            .iter()
            .enumerate()
            .map(|(i, g)| (g.to_string(), 10 - i))
            .collect();

        GenreAssignment {
            genres: genres.into_iter().map(String::from).collect(),
            genre_scores,
            article,
        }
    }

    #[test]
    fn evidence_bundle_groups_by_genre() {
        let job_id = Uuid::new_v4();
        let assignments = vec![
            create_assignment("art-1", vec!["ai", "tech"], vec!["Sentence 1."], "en"),
            create_assignment(
                "art-2",
                vec!["tech"],
                vec!["Sentence 2.", "Sentence 3."],
                "en",
            ),
            create_assignment("art-3", vec!["ai"], vec!["Sentence 4."], "ja"),
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

        // techジャンルには2つの記事（art-1とart-2）
        let tech_corpus = evidence.get_corpus("tech").unwrap();
        assert_eq!(tech_corpus.articles.len(), 2);
        assert_eq!(tech_corpus.total_sentences, 3);
    }

    #[test]
    fn corpus_metadata_tracks_languages() {
        let job_id = Uuid::new_v4();
        let assignments = vec![
            create_assignment("art-1", vec!["ai"], vec!["Sentence 1."], "en"),
            create_assignment("art-2", vec!["ai"], vec!["Sentence 2."], "en"),
            create_assignment("art-3", vec!["ai"], vec!["Sentence 3."], "ja"),
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
    }

    #[test]
    fn evidence_bundle_utility_methods() {
        let job_id = Uuid::new_v4();
        let assignments = vec![
            create_assignment("art-1", vec!["ai"], vec!["S1.", "S2."], "en"),
            create_assignment("art-2", vec!["tech"], vec!["S3."], "en"),
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
    }

    #[test]
    fn evidence_article_includes_genre_scores() {
        let job_id = Uuid::new_v4();
        let assignments = vec![create_assignment(
            "art-1",
            vec!["ai", "tech"],
            vec!["Sentence."],
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
}
