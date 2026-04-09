# Obsidian Vault 構造ガイド

## ディレクトリ構成

| ディレクトリ | 内容 | アクセス方法 |
|---|---|---|
| `ADR/` | Architecture Decision Records (000001〜最新) | grep / mcp__obsidian__view |
| `plan/` | 実装計画・正規コントラクト | mcp__obsidian__view |
| `runbooks/` | 運用手順 | mcp__obsidian__get_workspace_files → view |
| `review/` | 監査レポート・是正指示 | mcp__obsidian__view |
| `daily/` | デイリーノート (YYYY-MM-DD.md) | get_workspace_files → 最新を view |
| `services/` | マイクロサービスドキュメント | mcp__obsidian__view |

## 検索パターン

| 探したいもの | 検索先 | 操作 |
|---|---|---|
| 特定サービスの設計判断 | ADR/ (frontmatter: affected_services) | grep → view |
| Knowledge Home の契約 | plan/knowledge-home-phase0-canonical-contract.md | view |
| 全体実装計画 | plan/alt_knowledge_home_phase_plan.md | view |
| イミュータブルデータモデル設計 | plan/IMPL_BASE.md | view |
| フェーズ別詳細 | plan/IMPL_PHASE{1-6}.md | view |
| 運用手順・制約 | runbooks/ | get_workspace_files → view |
| 既知の問題・是正指示 | review/ | view |
| 直近の作業文脈 | daily/ | get_workspace_files → 最新を view |
| サービス仕様 | services/ | view |

## ADR frontmatter フィールド

```yaml
title: ADR タイトル
date: YYYY-MM-DD
status: proposed | accepted | deprecated | superseded
tags: [architecture, database, ...]
affected_services: [alt-backend, alt-frontend-sv, ...]
aliases: [ADR-NNN, ADR-000NNN]
```

## ADR タグ一覧

フィルタ可能な主要分類:

- **アーキテクチャ**: architecture, clean-architecture, connect-rpc
- **データ**: database, migration, pgbouncer
- **レイヤ**: frontend, backend, api
- **AI/ML**: ai, rag, recap
- **インフラ**: docker, networking, ci-cd
- **品質**: performance, security, testing, refactoring, bugfix
- **運用**: monitoring, logging
- **その他**: rss, search, caching, authentication, nats, queue, 3d-graphics

## サービス名一覧

affected_services でフィルタ可能:

- alt-backend, alt-frontend-sv, alt-butterfly-facade
- pre-processor, search-indexer, mq-hub
- news-creator, tag-generator, recap-worker, recap-subworker
- rag-orchestrator, auth-hub, auth-token-manager
- rask-log-aggregator, rask-log-forwarder
- metrics, recap-evaluator, alt-perf
