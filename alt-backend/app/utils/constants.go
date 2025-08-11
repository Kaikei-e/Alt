package utils

// DUMMY_USER_ID is a fixed user ID used for single-user mode read status functionality
// This constant is used to maintain compatibility with the database schema that requires user_id
// while keeping the current API structure intact
const DUMMY_USER_ID = "00000000-0000-0000-0000-000000000001"