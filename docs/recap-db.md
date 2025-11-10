# Recap Database Schema Contract

This document outlines the schema for the `recap_db` PostgreSQL database, which stores data related to the Recap Worker's processing of RSS feed articles.

## Tables

### `recap_jobs`
Stores information about each recap job, including its status, start/end times, and associated metadata.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `UUID`       | `PRIMARY KEY`       | Unique identifier for the recap job              |
| `status`         | `TEXT`       | `NOT NULL`          | Current status of the job (e.g., 'pending', 'in_progress', 'completed', 'failed') |
| `created_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the job was created               |
| `updated_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the job was last updated          |
| `article_id`     | `UUID`       | `NOT NULL`, `UNIQUE`| ID of the article being recapped                 |
| `genre`          | `TEXT`       | `NOT NULL`          | Genre of the article                             |
| `prompt_version` | `TEXT`       | `NOT NULL`          | Version of the prompt used for the LLM           |

### `recap_sections`
Stores the generated recap sections for each article.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `UUID`       | `PRIMARY KEY`       | Unique identifier for the recap section          |
| `job_id`         | `UUID`       | `NOT NULL`, `FOREIGN KEY (recap_jobs.id)` | ID of the associated recap job                   |
| `section_type`   | `TEXT`       | `NOT NULL`          | Type of section (e.g., 'summary', 'key_points')  |
| `content`        | `TEXT`       | `NOT NULL`          | The generated recap content                      |
| `created_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the section was created           |
| `updated_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the section was last updated      |

### `recap_sources`
Stores the sources (citations) used in the recap sections.

| Column Name      | Type         | Constraints         | Description                                      |
|------------------|--------------|---------------------|--------------------------------------------------|
| `id`             | `UUID`       | `PRIMARY KEY`       | Unique identifier for the source                 |
| `section_id`     | `UUID`       | `NOT NULL`, `FOREIGN KEY (recap_sections.id)` | ID of the associated recap section               |
| `source_text`    | `TEXT`       | `NOT NULL`          | The original text from the source                |
| `start_offset`   | `INTEGER`    | `NOT NULL`          | Start character offset in the original article   |
| `end_offset`     | `INTEGER`    | `NOT NULL`          | End character offset in the original article     |
| `created_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the source was recorded           |
| `updated_at`     | `TIMESTAMPZ` | `NOT NULL`          | Timestamp when the source was last updated       |
