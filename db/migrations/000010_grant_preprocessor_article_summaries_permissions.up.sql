-- Grant permissions on article_summaries table to the preprocessor user
GRANT SELECT, INSERT, UPDATE ON TABLE article_summaries TO pre_processor_user;