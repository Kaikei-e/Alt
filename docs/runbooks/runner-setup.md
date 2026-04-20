---
title: Self-hosted GitHub Actions runner セットアップ (alt-prod / alt-builder)
date: 2026-04-20
tags:
  - runbooks
  - ci-cd
  - deploy
  - security
---
# Self-hosted GitHub Actions runner セットアップ

alt-deploy の `release-deploy.yaml` が使う 2 つの self-hosted runner (`alt-builder` ラベル、`alt-prod` ラベル) の初期セットアップと、依存ツールの管理手順を集約する。ADR-000763 で定義された 2-machine split を前提とする (実ホストの割り当ては内部運用文書に分離)。

## TL;DR

| Runner label | 必要ツール | 導入タイミング |
|--------------|------------|----------------|
| `alt-builder` | Go, Rust, Python 3, Docker, libpact_ffi, pact-broker-cli | runner 登録時 |
| `alt-prod` | Docker, **Ansible (ansible-core + community.docker)** | runner 登録時 |

**deploy 経路で外部 fetch を発生させない** 方針。deploy-time install はすべて pre-installed を前提に check-only に倒す (A03 supply chain 最小化)。

## 1. alt-prod runner: Ansible 導入

### Why Ansible

ADR-000811 により `release-deploy.yaml` の `deploy` job は `community.docker.docker_compose_v2` module 経由で per-service roll + reconcile を宣言的に実行する。runner には **ansible-core と community.docker collection** が pre-install されている必要がある。

### Install / upgrade / verify は playbook 経由で宣言的に

ansible-core 自体は自己インストールできないため、**2 段 bootstrap**:

| Phase | 手段 | 対象 |
|-------|------|------|
| 1 | shell 1 行 | ansible-core 本体 (pipx 経由) |
| 2 | playbook | community.docker collection / pin version / SHA 記録 / drift 検知 |

Phase 2 は `alt-deploy/playbooks/setup-runner.yml` が真実源。**install / upgrade / verify すべて同じ playbook** を使う (CI の check step も同じ playbook を `--check` モードで回す)。

```bash
# ssh 先: alt-prod runner host (runner サービスユーザとして実行)
# 監査ログに残す
script -a /var/log/alt-prod-bootstrap.log

# Phase 1: pipx + ansible-core (shell、一度きり、PEP 668 対応)
python3 -m pip install --user --break-system-packages pipx
python3 -m pipx ensurepath
source ~/.bashrc
pipx install 'ansible-core==2.18.6'

# Phase 2: community.docker collection + SHA 記録 (以降の更新も同じコマンド)
cd ~/actions-runner/_work/alt-deploy/alt-deploy    # workflow checkout dir
ansible-playbook playbooks/setup-runner.yml -c local -i localhost,

# 完了確認 — 同じ playbook を --check で回すと冪等性が verify される
ansible-playbook playbooks/setup-runner.yml -c local -i localhost, --check

exit   # script -a を閉じる
```

### Pin version の更新フロー

`playbooks/setup-runner.yml` 先頭の `vars:` ブロックが唯一の pin 源:

```yaml
vars:
  ansible_core_version: "2.18.6"
  community_docker_version: "4.1.0"
```

更新手順:

1. [community.docker CHANGELOG](https://github.com/ansible-collections/community.docker/blob/main/CHANGELOG.rst) を quarterly で確認
2. `playbooks/setup-runner.yml` の vars を編集して PR / コミット
3. alt-prod host で `pipx upgrade ansible-core` → `ansible-playbook playbooks/setup-runner.yml -c local -i localhost,` を実行
4. playbook の最後の Summary task が新 version + 新 SHA を出す。runbook / 監査ログに貼る

### Security posture

- `pipx` は各 package 用の独立 virtualenv を作るため、system Python 環境を汚染しない
- `--break-system-packages` は pipx 自体の bootstrap のみ使用 (PEP 668)、以降は pipx 管理下
- deploy job は **check-only** で走る (install はしない)。PyPI / Galaxy への network 依存が deploy 経路から外れる
- runner user は非 root。Docker group 権限は持つが、Ansible playbook も user として走る
- 初回 install 時の supply chain リスクを最小化するため、operator 監督下で SHA 記録を残し、以降の drift 検出を可能にする

## 2. alt-prod runner: 健全性チェック

release-deploy の `deploy` job は同じ `setup-runner.yml` を `--check` モードで回す:

```bash
ansible-playbook playbooks/setup-runner.yml -c local -i localhost, --check
```

非 Ansible な前提 (docker, env / secrets staging) は従来どおり shell で:

```bash
command -v docker >/dev/null && echo OK-docker
[ -r "$HOME/alt-env/.env" ] && echo OK-env
[ -d "$HOME/alt-secrets" ] && echo OK-secrets
```

playbook が exit 0 + 上記 shell すべて OK で runner 設定完了。

## 3. alt-builder runner

release-deploy の `build` / `pact-publish` / `gate` / `e2e` jobs が走る。以下が必要:

- Go 1.26+ (`/usr/local/go/bin` に通す)
- Rust + Cargo (`~/.cargo/bin`)
- Python 3.14+
- Docker + Buildx
- `libpact_ffi.so` (`/usr/local/lib/libpact_ffi.so` or `~/.pact/lib/`)
- `pact-broker-cli` (Rust 版、`pact-broker-cli>=0.6.3`)
- `$HOME/alt-secrets/pact_broker_basic_auth_password.txt` (broker 認証)

既存手順は ADR-000763 のコメントに散在。必要なら本 runbook に別 section で集約する (backlog)。

## 4. Runner PATH の永続化

GHA self-hosted runner は `runner` user の `.profile` / `.bashrc` を sourcing しないことがある。`~/.local/bin` を PATH に入れるには以下のいずれか:

```bash
# (a) runner の .env ファイルに追記 (推奨)
echo "PATH=$HOME/.local/bin:$PATH" >> ~/actions-runner/.env

# (b) systemd unit なら Environment 直書き
# /etc/systemd/system/actions.runner.*.service を sudo systemctl edit して
# [Service]
# Environment="PATH=${HOME}/.local/bin:/usr/local/sbin:..."
#   ※ systemd は $HOME を展開しないので、実 PATH は上記のどちらかで明示展開する

# runner サービス再起動で反映
sudo systemctl restart actions.runner.*
```

`.env` 方式のほうが non-sudo で済むため推奨。

## 5. インシデント後の確認

2026-04-20 release-deploy run 24658313584 は **alt-prod runner に pip が入っておらず、per-job install が exit 127 で失敗** → deploy abort → production partial-deploy の連鎖を起こした。本 runbook の install 手順が未実施だと同クラスの障害が再発する。

導入後の最初の workflow_dispatch で `deploy` job の `Verify ansible-playbook is available` step が OK になっていれば設定は完了。

## 参考

- [[000740]] Pact Broker 恒常化
- [[000763]] 2-machine split pull-deploy
- [[000809]] compose healthcheck invariant
- [[000810]] pact-check.sh manual/consumer 分離
- [[000811]] (予定) release-deploy を Ansible 宣言的ロール化
- [pipx 公式](https://pipx.pypa.io/) — PEP 668 と pipx の位置づけ
- [community.docker CHANGELOG](https://github.com/ansible-collections/community.docker/blob/main/CHANGELOG.rst) — 更新時に確認
