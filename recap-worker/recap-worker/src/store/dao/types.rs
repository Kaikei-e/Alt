/// JobStatus - リキャップジョブの状態を表す列挙型
#[derive(Debug, Clone, PartialEq, sqlx::Type)]
#[sqlx(type_name = "text", rename_all = "lowercase")]
pub enum JobStatus {
    Pending,
    Running,
    Completed,
    Failed,
}

impl AsRef<str> for JobStatus {
    fn as_ref(&self) -> &str {
        match self {
            JobStatus::Pending => "pending",
            JobStatus::Running => "running",
            JobStatus::Completed => "completed",
            JobStatus::Failed => "failed",
        }
    }
}
