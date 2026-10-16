-- Extensions and shared helpers
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Application role: RLS IS enforced for this role (it does not own tables).
-- The role itself is created by deployments/docker/postgres-init.sh (needs
-- superuser); here we only make sure grants exist when it is present.
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'wallet_app') THEN
    CREATE ROLE wallet_app LOGIN PASSWORD 'wallet_app_dev_password';
  END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO wallet_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO wallet_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO wallet_app;

-- updated_at trigger helper
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
