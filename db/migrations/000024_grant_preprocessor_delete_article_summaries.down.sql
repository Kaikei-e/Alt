-- Revoke DELETE permission on article_summaries table from the preprocessor user
REVOKE DELETE ON TABLE article_summaries FROM pre_processor_user;