use std::time::Duration;

use anyhow::{Context, Result};
use reqwest::{Client, Url};
use serde::Deserialize;
use uuid::Uuid;

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    client: Client,
    base_url: Url,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
pub(crate) struct SubworkerArticle {
    pub(crate) id: Uuid,
    pub(crate) title: String,
    pub(crate) body: String,
    #[serde(default)]
    pub(crate) language: Option<String>,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
pub(crate) struct SubworkerCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<SubworkerArticle>,
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(10))
            .build()
            .context("failed to build subworker client")?;

        let base_url = Url::parse(&endpoint.into()).context("invalid subworker base URL")?;

        Ok(Self { client, base_url })
    }

    pub(crate) async fn ping(&self) -> Result<()> {
        let url = self
            .base_url
            .join("health")
            .context("failed to build subworker health URL")?;

        self.client
            .get(url)
            .send()
            .await
            .context("subworker health request failed")?
            .error_for_status()
            .context("subworker health endpoint returned error status")?;

        Ok(())
    }

    pub(crate) async fn fetch_corpus(&self, job_id: Uuid) -> Result<SubworkerCorpus> {
        let url = self
            .base_url
            .join(&format!("jobs/{job_id}"))
            .context("failed to build subworker jobs URL")?;

        let response = self
            .client
            .get(url)
            .send()
            .await
            .context("subworker corpus request failed")?
            .error_for_status()
            .context("subworker corpus endpoint returned error status")?;

        response
            .json::<SubworkerCorpus>()
            .await
            .context("failed to deserialize subworker corpus response")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn ping_succeeds_for_ok_response() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri()).expect("client should build");

        client.ping().await.expect("ping should succeed");
    }

    #[tokio::test]
    async fn ping_fails_on_error_status() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(500))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri()).expect("client should build");

        let error = client.ping().await.expect_err("ping should fail");
        assert!(error.to_string().contains("error status"));
    }

    #[tokio::test]
    async fn fetch_corpus_returns_articles() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let body = serde_json::json!({
            "job_id": job_id,
            "articles": [
                {
                    "id": Uuid::new_v4(),
                    "title": "Title",
                    "body": "Body",
                    "language": "en"
                }
            ]
        });

        Mock::given(method("GET"))
            .and(path(format!("/jobs/{job_id}").as_str()))
            .respond_with(ResponseTemplate::new(200).set_body_json(body.clone()))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri()).expect("client should build");
        let corpus = client
            .fetch_corpus(job_id)
            .await
            .expect("fetch should succeed");

        let expected: SubworkerCorpus = serde_json::from_value(body).expect("valid body");
        assert_eq!(corpus, expected);
    }

    #[tokio::test]
    async fn fetch_corpus_propagates_error_status() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();

        Mock::given(method("GET"))
            .and(path(format!("/jobs/{job_id}").as_str()))
            .respond_with(ResponseTemplate::new(404))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri()).expect("client should build");
        let error = client.fetch_corpus(job_id).await.expect_err("should fail");
        assert!(error.to_string().contains("error status"));
    }
}
