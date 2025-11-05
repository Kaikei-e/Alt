use async_trait::async_trait;
use unicode_normalization::UnicodeNormalization;
use uuid::Uuid;
use whatlang::detect;

use crate::scheduler::JobContext;

use super::fetch::{FetchedArticle, FetchedCorpus};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PreprocessedArticle {
    pub(crate) id: Uuid,
    pub(crate) title: String,
    pub(crate) body: String,
    pub(crate) language: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PreprocessedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<PreprocessedArticle>,
}

#[async_trait]
pub(crate) trait PreprocessStage: Send + Sync {
    async fn preprocess(
        &self,
        job: &JobContext,
        corpus: FetchedCorpus,
    ) -> anyhow::Result<PreprocessedCorpus>;
}

#[derive(Debug, Clone)]
pub(crate) struct TextPreprocessStage;

impl TextPreprocessStage {
    pub(crate) fn new() -> Self {
        Self
    }
}

impl Default for TextPreprocessStage {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl PreprocessStage for TextPreprocessStage {
    async fn preprocess(
        &self,
        job: &JobContext,
        corpus: FetchedCorpus,
    ) -> anyhow::Result<PreprocessedCorpus> {
        let mut articles = Vec::with_capacity(corpus.articles.len());

        for article in corpus.articles {
            if let Some(processed) = preprocess_article(article) {
                articles.push(processed);
            }
        }

        Ok(PreprocessedCorpus {
            job_id: job.job_id,
            articles,
        })
    }
}

fn preprocess_article(article: FetchedArticle) -> Option<PreprocessedArticle> {
    let normalized = article.body.nfc().collect::<String>();
    let trimmed = normalized.trim();
    if trimmed.is_empty() {
        return None;
    }

    let language = article
        .language
        .or_else(|| detect(trimmed).map(|info| info.lang().code().to_string()))
        .unwrap_or_else(|| "und".to_string());

    Some(PreprocessedArticle {
        id: article.id,
        title: article.title,
        body: trimmed.to_string(),
        language,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use rstest::rstest;

    fn article(body: &str, language: Option<&str>) -> FetchedArticle {
        FetchedArticle {
            id: Uuid::new_v4(),
            title: "Title".into(),
            body: body.into(),
            language: language.map(std::string::ToString::to_string),
        }
    }

    #[rstest]
    #[case("  正規化テキスト  ", Some("ja"))]
    #[case("Text with spaces", None)]
    fn preprocess_article_trims_and_detects_language(
        #[case] body: &str,
        #[case] language: Option<&str>,
    ) {
        let fetched = article(body, language);
        let result = preprocess_article(fetched).expect("article should remain");
        assert_eq!(result.body, body.trim());
        if let Some(lang) = language {
            assert_eq!(result.language, lang);
        } else {
            assert!(!result.language.is_empty());
        }
    }

    #[test]
    fn preprocess_article_drops_empty() {
        assert!(preprocess_article(article("   ", None)).is_none());
    }

    #[tokio::test]
    async fn preprocess_filters_empty_articles() {
        let stage = TextPreprocessStage::new();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = FetchedCorpus {
            job_id: job.job_id,
            articles: vec![article("Body", None), article("   ", None)],
        };

        let result = stage
            .preprocess(&job, corpus)
            .await
            .expect("preprocess succeeds");

        assert_eq!(result.articles.len(), 1);
    }
}
