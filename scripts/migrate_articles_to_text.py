#!/usr/bin/env python3
"""
高効率な記事HTML→テキスト移行スクリプト

既存のarticlesテーブルのHTMLデータをテキスト抽出済みデータに移行します。
- 並列処理で高速化
- バッチ更新で効率化
- 進捗表示とエラーハンドリング
"""

import argparse
import json
import os
import re
import sys
import time
import traceback
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path

import psycopg2
import psycopg2.extensions
import psycopg2.extras
from bs4 import BeautifulSoup
from tqdm import tqdm

MIN_EXTRACTED_TEXT_LEN = 10
CONN_KEEPALIVE_INTERVAL_SEC = 30

# readability-lxmlのインポート（オプショナル）
try:
    from readability.readability import Document as ReadabilityDocument

    READABILITY_AVAILABLE = True
except ImportError:
    READABILITY_AVAILABLE = False
    ReadabilityDocument = None


@dataclass
class ArticleRecord:
    """記事レコード"""

    id: str
    title: str
    content: str
    url: str


class ArticleTextExtractor:
    """記事テキスト抽出器（Goのhtml_parserと同じロジック）"""

    @staticmethod
    def _extract_next_data_body(soup: BeautifulSoup) -> str | None:
        """Next.js __NEXT_DATA__ から記事本文HTMLを取得"""
        next_data = soup.find("script", id="__NEXT_DATA__")
        if not next_data:
            return None
        try:
            data = json.loads(next_data.string)
            if "props" in data and "pageProps" in data["props"]:
                page_props = data["props"]["pageProps"]
                if "article" in page_props:
                    article_data = page_props["article"]
                    if "bodyHtml" in article_data:
                        body_html = article_data["bodyHtml"]
                        if body_html:
                            return body_html
        except (json.JSONDecodeError, KeyError, TypeError):
            pass
        return None

    @staticmethod
    def _remove_unwanted_tags(soup: BeautifulSoup) -> None:
        """ナビゲーション・埋め込み・SNS・コメント・メタタグを除去"""
        for tag in soup.find_all(
            ["head", "script", "style", "noscript", "title", "aside", "nav", "header", "footer"]
        ):
            tag.decompose()

        for tag in soup.find_all(["iframe", "embed", "object", "video", "audio", "canvas"]):
            tag.decompose()

        for selector in [
            '[class*="social"]',
            '[class*="share"]',
            '[class*="twitter"]',
            '[class*="facebook"]',
            '[class*="instagram"]',
            '[class*="linkedin"]',
            '[id*="social"]',
            '[id*="share"]',
            '[id*="twitter"]',
            '[id*="facebook"]',
        ]:
            for tag in soup.select(selector):
                tag.decompose()

        for selector in [
            '[class*="comment"]',
            '[id*="comment"]',
            '[class*="discussion"]',
            '[id*="discussion"]',
        ]:
            for tag in soup.select(selector):
                tag.decompose()

        for tag in soup.find_all(["meta"]):
            tag.decompose()
        for tag in soup.find_all("link", rel=["stylesheet", "preload", "prefetch", "dns-prefetch"]):
            tag.decompose()

    @staticmethod
    def _strip_inline_attributes(soup: BeautifulSoup) -> None:
        """インラインスタイルとイベントハンドラー属性を除去"""
        for tag in soup.find_all(True):
            if "style" in tag.attrs:
                del tag.attrs["style"]
            event_attrs = [attr for attr in tag.attrs if attr.startswith("on")]
            for attr in event_attrs:
                del tag.attrs[attr]

    @staticmethod
    def extract_text(html_content: str) -> str:
        """
        HTMLから記事テキストを抽出

        GoのExtractArticleTextと同じロジック:
        1. Next.js __NEXT_DATA__ のチェック
        2. 不要要素の除去
        3. go-readability相当の処理
        4. パラグラフ抽出
        """
        if not html_content or not html_content.strip():
            return ""

        # 既にテキストの場合はそのまま返す
        if "<" not in html_content:
            return html_content.strip()

        try:
            soup = BeautifulSoup(html_content, "lxml")

            # 1. Next.js __NEXT_DATA__ のチェック
            next_body = ArticleTextExtractor._extract_next_data_body(soup)
            if next_body is not None:
                return ArticleTextExtractor._extract_paragraphs(next_body)

            # 2. 不要要素の除去（Goのロジックと同じ）
            ArticleTextExtractor._remove_unwanted_tags(soup)
            ArticleTextExtractor._strip_inline_attributes(soup)

            # 3. readability-lxmlで記事抽出（利用可能な場合）
            cleaned_html = str(soup)
            if READABILITY_AVAILABLE:
                try:
                    doc = ReadabilityDocument(cleaned_html)
                    summary_html = doc.summary()
                    if summary_html and len(summary_html.strip()) > 0:
                        # readabilityが成功した場合、そのHTMLからパラグラフ抽出
                        return ArticleTextExtractor._extract_paragraphs(summary_html)
                except (ValueError, TypeError, AttributeError) as e:
                    sys.stderr.write(f"readability failed, falling back: {e}\n")

            # 4. フォールバック: パラグラフ抽出
            return ArticleTextExtractor._extract_paragraphs(cleaned_html)

        except (ValueError, TypeError, AttributeError) as e:
            sys.stderr.write(f"HTML extraction fallback: {e}\n")
            return ArticleTextExtractor._simple_strip_tags(html_content)

    @staticmethod
    def _collect_tag_texts(
        soup: BeautifulSoup, tags: list[str] | str, min_len: int = 0
    ) -> list[str]:
        """指定タグ群から空でないテキストを抽出"""
        texts: list[str] = []
        for tag in soup.find_all(tags):
            text = tag.get_text(strip=True)
            if text and len(text) > min_len:
                texts.append(text)
        return texts

    @staticmethod
    def _extract_paragraphs(html: str) -> str:
        """パラグラフを抽出して結合"""
        try:
            soup = BeautifulSoup(html, "lxml")
            paragraphs: list[str] = []

            for tags in (["h1", "h2", "h3", "h4", "h5", "h6"], "p", ["pre", "code"], "li"):
                paragraphs.extend(ArticleTextExtractor._collect_tag_texts(soup, tags))

            # 構造化コンテンツが見つからない場合
            if not paragraphs:
                paragraphs.extend(
                    ArticleTextExtractor._collect_tag_texts(
                        soup, ["div", "article", "section"], MIN_EXTRACTED_TEXT_LEN
                    )
                )

            if paragraphs:
                return "\n\n".join(paragraphs)
            return ArticleTextExtractor._simple_strip_tags(html)

        except (ValueError, TypeError, AttributeError) as e:
            sys.stderr.write(f"paragraph extraction fallback: {e}\n")
            return ArticleTextExtractor._simple_strip_tags(html)

    @staticmethod
    def _simple_strip_tags(html: str) -> str:
        """シンプルなタグ除去"""
        soup = BeautifulSoup(html, "lxml")
        # scriptとstyleを除去
        for tag in soup.find_all(["script", "style"]):
            tag.decompose()
        text = soup.get_text(separator=" ", strip=True)
        # 空白を正規化
        text = re.sub(r"\s+", " ", text)
        return text.strip()


class DatabaseManager:
    """データベース管理"""

    def __init__(self, dsn: str) -> None:
        self.dsn = dsn
        self.conn: psycopg2.extensions.connection | None = None

    def connect(self) -> None:
        """データベースに接続"""
        self.conn = psycopg2.connect(
            self.dsn,
            connect_timeout=10,
            keepalives=1,
            keepalives_idle=30,
            keepalives_interval=10,
            keepalives_count=5,
        )
        self.conn.autocommit = False

    def ensure_connection(self) -> None:
        """接続が有効か確認し、必要に応じて再接続"""
        if self.conn is None:
            self.connect()
            return

        try:
            # 接続の生存確認
            cur = self.conn.cursor()
            cur.execute("SELECT 1")
            cur.fetchone()
            cur.close()
        except (psycopg2.OperationalError, psycopg2.InterfaceError):
            # 接続が切れている場合は再接続
            try:
                self.conn.close()
            except (psycopg2.Error, OSError) as e:
                sys.stderr.write(f"close before reconnect: {e}\n")
            self.connect()

    def close(self) -> None:
        """接続を閉じる"""
        if self.conn:
            try:
                self.conn.close()
            except (psycopg2.Error, OSError) as e:
                sys.stderr.write(f"close error: {e}\n")
            self.conn = None

    def get_articles_batch(self, last_id: str | None, limit: int) -> list[ArticleRecord]:
        """記事をバッチで取得（HTMLを含むもののみ、keysetページネーション）

        OFFSETベースのページネーションは、バッチ更新のたびに
        `content LIKE '<%'` にマッチする行の集合が縮むため、
        次バッチで未処理の行を読み飛ばしてしまう。id を起点にした
        keysetページネーションであれば、常に「前回見た最後のidより後」
        を問い合わせるため、集合が縮んでも取りこぼしが起きない。
        """
        self.ensure_connection()
        cur = self.conn.cursor()
        try:
            if last_id is None:
                cur.execute(
                    """
                    SELECT id, title, content, url
                    FROM articles
                    WHERE content LIKE %s
                    ORDER BY id
                    LIMIT %s
                """,
                    ("<%", limit),
                )
            else:
                cur.execute(
                    """
                    SELECT id, title, content, url
                    FROM articles
                    WHERE content LIKE %s AND id > %s
                    ORDER BY id
                    LIMIT %s
                """,
                    ("<%", last_id, limit),
                )

            rows = cur.fetchall()
            return [
                ArticleRecord(
                    id=str(row[0]), title=row[1] or "", content=row[2] or "", url=row[3] or ""
                )
                for row in rows
            ]
        finally:
            cur.close()

    def count_html_articles(self) -> int:
        """HTMLを含む記事の総数を取得"""
        self.ensure_connection()
        cur = self.conn.cursor()
        try:
            cur.execute("SELECT COUNT(*) FROM articles WHERE content LIKE '<%'")
            return cur.fetchone()[0]
        finally:
            cur.close()

    def update_article_batch(self, updates: list[tuple[str, str]], max_retries: int = 3) -> None:
        """記事をバッチで更新（再接続機能付き）"""
        for attempt in range(max_retries):
            try:
                self.ensure_connection()
                cur = self.conn.cursor()
                try:
                    psycopg2.extras.execute_batch(
                        cur,
                        "UPDATE articles SET content = %s WHERE id = %s",
                        [(content, article_id) for article_id, content in updates],
                        page_size=100,
                    )
                    self.conn.commit()
                except (psycopg2.OperationalError, psycopg2.InterfaceError):
                    # 接続エラーの場合はロールバックして再接続を試みる
                    try:
                        self.conn.rollback()
                    except (psycopg2.Error, OSError) as e:
                        sys.stderr.write(f"rollback failed: {e}\n")
                    if attempt < max_retries - 1:
                        sys.stderr.write(
                            f"接続エラーが発生しました。再接続を試みます... (試行 {attempt + 1}/{max_retries})\n"
                        )
                        time.sleep(1)  # 少し待ってから再接続
                        try:
                            if self.conn is not None:
                                self.conn.close()
                        except (psycopg2.Error, OSError):
                            pass
                        self.conn = None  # 接続を無効化して次回再接続させる
                        continue
                    raise  # 最後の試行でも失敗した場合は例外を再発生
                else:
                    return  # 成功したら終了
                finally:
                    cur.close()
            except Exception as e:
                if attempt == max_retries - 1:
                    raise  # 最後の試行で失敗した場合は例外を再発生
                sys.stderr.write(
                    f"バッチ更新エラー: {e}。再試行します... (試行 {attempt + 1}/{max_retries})\n"
                )
                time.sleep(1)


def process_article(article: ArticleRecord) -> tuple[str, str, bool]:
    """
    記事を処理してテキスト抽出

    Returns:
        (article_id, extracted_text, success)
    """
    try:
        extracted = ArticleTextExtractor.extract_text(article.content)
    except Exception as e:  # noqa: BLE001 — per-article extraction fallback keeps batch migration running
        sys.stderr.write(f"Error processing article {article.id}: {e}\n")
        return (article.id, article.content, False)
    else:
        if not extracted or len(extracted.strip()) < MIN_EXTRACTED_TEXT_LEN:
            return (article.id, article.content, False)  # 抽出失敗、元のコンテンツを保持
        return (article.id, extracted, True)


def load_env_vars() -> dict[str, str]:
    """環境変数または.envファイルからデータベース接続情報を読み込む"""
    env_file = Path(__file__).parent.parent / ".env"
    env_vars = {}

    # .envファイルから読み込み
    if env_file.exists():
        with env_file.open(encoding="utf-8") as f:
            for raw_line in f:
                line = raw_line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    env_vars[key.strip()] = value.strip().strip('"').strip("'")

    # 環境変数で上書き
    for key in [
        "DB_HOST",
        "DB_PORT",
        "DB_USER",
        "DB_PASSWORD",
        "DB_PASSWORD_FILE",
        "DB_NAME",
        "POSTGRES_DB",
    ]:
        if key in os.environ:
            env_vars[key] = os.environ[key]

    return env_vars


def get_database_dsn() -> str:
    """データベース接続文字列を取得"""
    env_vars = load_env_vars()

    # ホスト設定（Docker外で実行する場合はlocalhostに変換）
    db_host = env_vars.get("DB_HOST", "localhost")
    if db_host == "db":
        db_host = "localhost"

    db_port = env_vars.get("DB_PORT", "5432")
    db_user = env_vars.get("DB_USER", "alt_appuser")
    db_name = env_vars.get("DB_NAME") or env_vars.get("POSTGRES_DB", "alt")

    # パスワードの取得
    db_password = env_vars.get("DB_PASSWORD", "")

    # パスワードファイルのサポート
    if not db_password:
        # 1. DB_PASSWORD_FILE環境変数から読み込み
        password_file = env_vars.get("DB_PASSWORD_FILE")
        if password_file:
            password_path = Path(password_file)
            if password_path.exists():
                try:
                    with password_path.open(encoding="utf-8") as f:
                        db_password = f.read().strip()
                except (OSError, UnicodeDecodeError) as e:
                    sys.stderr.write(f"警告: パスワードファイルの読み込みに失敗しました: {e}\n")

    if not db_password:
        # 2. secretsディレクトリから読み込みを試行
        script_dir = Path(__file__).parent
        repo_root = script_dir.parent
        secret_path = repo_root / "secrets" / "db_password.txt"
        if secret_path.exists():
            try:
                db_password = secret_path.read_text(encoding="utf-8").strip()
            except (OSError, UnicodeDecodeError) as e:
                sys.stderr.write(f"警告: secrets/db_password.txtの読み込みに失敗しました: {e}\n")

    if not db_password:
        sys.stderr.write("警告: データベースパスワードが設定されていません。\n")
        sys.stderr.write("以下のいずれかを設定してください:\n")
        sys.stderr.write("  - 環境変数 DB_PASSWORD\n")
        sys.stderr.write("  - 環境変数 DB_PASSWORD_FILE (ファイルパス)\n")
        sys.stderr.write("  - secrets/db_password.txt ファイル\n")

    return f"postgresql://{db_user}:{db_password}@{db_host}:{db_port}/{db_name}?sslmode=prefer"


@dataclass
class _MigrationStats:
    """移行処理の集計カウンタ"""

    processed: int = 0
    updated: int = 0
    failed: int = 0
    total_original_size: int = 0
    total_extracted_size: int = 0


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="記事HTML→テキスト移行スクリプト")
    parser.add_argument(
        "--batch-size", type=int, default=1000, help="バッチサイズ（デフォルト: 1000）"
    )
    parser.add_argument("--workers", type=int, default=8, help="並列処理数（デフォルト: 8）")
    parser.add_argument("--dry-run", action="store_true", help="ドライラン（実際には更新しない）")
    parser.add_argument("--limit", type=int, help="処理する記事数の上限（テスト用）")
    parser.add_argument(
        "--dsn",
        type=str,
        help="PostgreSQL接続文字列（例: postgresql://user:pass@host:port/dbname）",
    )
    return parser.parse_args()


def _process_batch(
    articles: list[ArticleRecord],
    workers: int,
    stats: _MigrationStats,
    pbar: tqdm,
) -> list[tuple[str, str]]:
    """1バッチの記事を並列処理し、更新対象リストを返す"""
    batch_updates: list[tuple[str, str]] = []
    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = {executor.submit(process_article, article): article for article in articles}

        for future in as_completed(futures):
            article_id, extracted_text, success = future.result()
            article = futures[future]

            stats.processed += 1
            stats.total_original_size += len(article.content)
            stats.total_extracted_size += len(extracted_text)

            if success:
                stats.updated += 1
                batch_updates.append((article_id, extracted_text))
            else:
                stats.failed += 1

            pbar.update(1)
            pbar.set_postfix(
                {
                    "更新": stats.updated,
                    "失敗": stats.failed,
                    "削減率": f"{(1 - stats.total_extracted_size / stats.total_original_size) * 100:.1f}%"
                    if stats.total_original_size > 0
                    else "0%",
                }
            )
    return batch_updates


def _run_migration(
    db: DatabaseManager, args: argparse.Namespace, total_count: int
) -> tuple[_MigrationStats, float]:
    """バッチ取得・抽出・更新ループを実行し、統計と経過時間を返す"""
    stats = _MigrationStats()
    start_time = time.time()
    last_connection_check = time.time()

    with tqdm(total=total_count, desc="処理中", unit="件") as pbar:
        last_id: str | None = None
        fetched_count = 0

        while fetched_count < total_count:
            current_time = time.time()
            if current_time - last_connection_check > CONN_KEEPALIVE_INTERVAL_SEC:
                db.ensure_connection()
                last_connection_check = current_time

            articles = db.get_articles_batch(last_id, args.batch_size)
            if not articles:
                break

            batch_updates = _process_batch(articles, args.workers, stats, pbar)

            if batch_updates and not args.dry_run:
                db.update_article_batch(batch_updates)

            last_id = articles[-1].id
            fetched_count += len(articles)

            if args.limit and fetched_count >= args.limit:
                break

    return stats, time.time() - start_time


def _print_summary(stats: _MigrationStats, elapsed_time: float, dry_run: bool) -> None:
    """移行結果のサマリを標準出力へ表示"""
    sys.stdout.write("\n" + "=" * 60 + "\n")
    sys.stdout.write("移行完了\n")
    sys.stdout.write("=" * 60 + "\n")
    sys.stdout.write(f"処理記事数: {stats.processed:,}件\n")
    sys.stdout.write(f"更新成功: {stats.updated:,}件\n")
    sys.stdout.write(f"更新失敗: {stats.failed:,}件\n")
    sys.stdout.write(f"処理時間: {elapsed_time:.2f}秒\n")
    sys.stdout.write(
        f"処理速度: {stats.processed / elapsed_time:.1f}件/秒\n" if elapsed_time > 0 else "N/A\n"
    )
    sys.stdout.write(f"元のサイズ: {stats.total_original_size / 1024 / 1024:.2f}MB\n")
    sys.stdout.write(f"抽出後サイズ: {stats.total_extracted_size / 1024 / 1024:.2f}MB\n")
    if stats.total_original_size > 0:
        reduction = (1 - stats.total_extracted_size / stats.total_original_size) * 100
        sys.stdout.write(f"削減率: {reduction:.2f}%\n")

    if dry_run:
        sys.stdout.write("\n*** ドライランモードでした。実際には更新していません ***\n")


def main() -> None:
    args = _parse_args()

    # データベース接続文字列の取得
    dsn = args.dsn or get_database_dsn()

    db = DatabaseManager(dsn)

    try:
        sys.stdout.write("データベースに接続中...\n")
        db.connect()

        sys.stdout.write("HTMLを含む記事数を取得中...\n")
        total_count = db.count_html_articles()
        sys.stdout.write(f"処理対象記事数: {total_count:,}件\n")

        if total_count == 0:
            sys.stdout.write("処理対象の記事がありません。\n")
            return

        if args.limit:
            total_count = min(total_count, args.limit)
            sys.stdout.write(f"制限により処理数: {total_count:,}件\n")

        if args.dry_run:
            sys.stdout.write("*** ドライランモード: 実際には更新しません ***\n")

        stats, elapsed_time = _run_migration(db, args, total_count)
        _print_summary(stats, elapsed_time, args.dry_run)

    except Exception as e:  # noqa: BLE001 — top-level CLI boundary logs traceback before DB cleanup and exit
        sys.stderr.write(f"エラーが発生しました: {e}\n")
        traceback.print_exc()
        sys.exit(1)

    finally:
        db.close()


if __name__ == "__main__":
    main()
