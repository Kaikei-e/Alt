CREATE TABLE IF NOT EXISTS declined_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    domain VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_declined_domains_user_id ON declined_domains(user_id);
CREATE INDEX IF NOT EXISTS idx_declined_domains_domain ON declined_domains(domain);
