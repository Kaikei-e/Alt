#![allow(dead_code)]

use anyhow::{Context, Result};
use sqlx::{PgConnection, Row};
use uuid::Uuid;

pub(crate) fn idempotency_key(job_id: Uuid, genre: &str) -> String {
    format!("{job_id}:{genre}")
}

/// UUIDを64ビットの整数キーに変換し、PostgreSQLのアドバイザリロックで使用できるようにする。
///
/// この関数はUUIDの最初の8バイトをビッグエンディアンのi64に変換します。
/// より安全なアプローチとして、PostgreSQL側でMD5ベースのハッシュ関数を使用することも可能です。
pub(crate) fn job_lock_key(job_id: Uuid) -> i64 {
    let bytes = job_id.as_bytes();

    i64::from_be_bytes([
        bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
    ])
}

/// PostgreSQLのアドバイザリトランザクションロックを取得する。
///
/// `pg_try_advisory_xact_lock`を使用してロックを取得します。
/// このロックはトランザクションが終了すると自動的に解放されます。
///
/// # Returns
/// - `Ok(true)`: ロックの取得に成功
/// - `Ok(false)`: ロックが既に他のセッションに保持されている
/// - `Err`: データベースエラー
///
/// # Errors
/// SQLクエリの実行に失敗した場合はエラーを返します。
pub async fn try_acquire_job_lock(conn: &mut PgConnection, job_id: Uuid) -> Result<bool> {
    let lock_key = job_lock_key(job_id);

    let row = sqlx::query("SELECT pg_try_advisory_xact_lock($1) as acquired")
        .bind(lock_key)
        .fetch_one(conn)
        .await
        .context("failed to execute pg_try_advisory_xact_lock")?;

    let acquired: bool = row
        .try_get("acquired")
        .context("failed to get lock acquisition result")?;

    Ok(acquired)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn job_lock_key_is_deterministic() {
        let uuid = Uuid::parse_str("550e8400-e29b-41d4-a716-446655440000").unwrap();
        let key1 = job_lock_key(uuid);
        let key2 = job_lock_key(uuid);
        assert_eq!(key1, key2);
    }

    #[test]
    fn different_uuids_produce_different_keys() {
        let uuid1 = Uuid::parse_str("550e8400-e29b-41d4-a716-446655440000").unwrap();
        let uuid2 = Uuid::parse_str("660e8400-e29b-41d4-a716-446655440001").unwrap();
        let key1 = job_lock_key(uuid1);
        let key2 = job_lock_key(uuid2);
        assert_ne!(key1, key2);
    }
}
