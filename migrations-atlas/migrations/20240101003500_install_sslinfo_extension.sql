-- Migration: install sslinfo extension
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000035_install_sslinfo_extension.up.sql

-- Install sslinfo extension for SSL status monitoring
-- INCIDENT 63 FIX: Resolve ssl_is_used() function not found error
-- Reference: https://www.postgresql.org/docs/current/sslinfo.html

CREATE EXTENSION IF NOT EXISTS sslinfo;

-- Verify installation by testing ssl_is_used function
-- This will return boolean indicating if current connection uses SSL
SELECT ssl_is_used() AS ssl_connection_status;
