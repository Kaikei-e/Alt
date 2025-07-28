-- Revoke permissions on article_summaries table from the preprocessor user
REVOKE ALL ON TABLE article_summaries FROM pre_processor_user;