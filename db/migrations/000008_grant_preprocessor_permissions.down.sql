-- Revoke preprocessor user permissions
REVOKE ALL ON TABLE feeds FROM pre_processor_user;
REVOKE ALL ON TABLE articles FROM pre_processor_user;