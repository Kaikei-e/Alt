#!/usr/bin/env python3
"""
高効率な記事HTML→テキスト移行スクリプト

既存のarticlesテーブルのHTMLデータをテキスト抽出済みデータに移行します。
- 並列処理で高速化
- バッチ更新で効率化
- 進捗表示とエラーハンドリング
"""

import os
import sys
import time
import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import List, Tuple, Optional, Dict
from dataclasses import dataclass
from pathlib import Path

import psycopg2
import psycopg2.extras
from psycopg2.extensions import connection as Connection
from bs4 import BeautifulSoup
from tqdm import tqdm

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
            soup = BeautifulSoup(html_content, 'lxml')

            # 1. Next.js __NEXT_DATA__ のチェック
            next_data = soup.find('script', id='__NEXT_DATA__')
            if next_data:
                try:
                    import json
                    data = json.loads(next_data.string)
                    if 'props' in data and 'pageProps' in data['props']:
                        page_props = data['props']['pageProps']
                        if 'article' in page_props:
                            article_data = page_props['article']
                            if 'bodyHtml' in article_data:
                                body_html = article_data['bodyHtml']
                                if body_html:
                                    return ArticleTextExtractor._extract_paragraphs(body_html)
                except (json.JSONDecodeError, KeyError, TypeError):
                    pass

            # 2. 不要要素の除去（Goのロジックと同じ）
            # ナビゲーション、ヘッダー、フッター
            for tag in soup.find_all(['head', 'script', 'style', 'noscript', 'title', 'aside', 'nav', 'header', 'footer']):
                tag.decompose()

            # メディアと埋め込みコンテンツ
            for tag in soup.find_all(['iframe', 'embed', 'object', 'video', 'audio', 'canvas']):
                tag.decompose()

            # ソーシャルメディア要素
            for selector in [
                '[class*="social"]', '[class*="share"]', '[class*="twitter"]',
                '[class*="facebook"]', '[class*="instagram"]', '[class*="linkedin"]',
                '[id*="social"]', '[id*="share"]', '[id*="twitter"]', '[id*="facebook"]'
            ]:
                for tag in soup.select(selector):
                    tag.decompose()

            # コメントセクション
            for selector in [
                '[class*="comment"]', '[id*="comment"]',
                '[class*="discussion"]', '[id*="discussion"]'
            ]:
                for tag in soup.select(selector):
                    tag.decompose()

            # メタデータとリソースリンク
            for tag in soup.find_all(['meta']):
                tag.decompose()
            for tag in soup.find_all('link', rel=['stylesheet', 'preload', 'prefetch', 'dns-prefetch']):
                tag.decompose()

            # インラインスタイルとイベントハンドラー属性の除去
            for tag in soup.find_all(True):
                # スタイル属性を除去
                if 'style' in tag.attrs:
                    del tag.attrs['style']
                # イベントハンドラー属性を除去
                event_attrs = [attr for attr in tag.attrs.keys() if attr.startswith('on')]
                for attr in event_attrs:
                    del tag.attrs[attr]

            # 3. readability-lxmlで記事抽出（利用可能な場合）
            if READABILITY_AVAILABLE:
                cleaned_html = str(soup)
                try:
                    doc = ReadabilityDocument(cleaned_html)
                    summary_html = doc.summary()
                    if summary_html and len(summary_html.strip()) > 0:
                        # readabilityが成功した場合、そのHTMLからパラグラフ抽出
                        return ArticleTextExtractor._extract_paragraphs(summary_html)
                except Exception:
                    pass

            # 4. フォールバック: パラグラフ抽出
            return ArticleTextExtractor._extract_paragraphs(cleaned_html)

        except Exception as e:
            # エラー時はシンプルなタグ除去
            return ArticleTextExtractor._simple_strip_tags(html_content)

    @staticmethod
    def _extract_paragraphs(html: str) -> str:
        """パラグラフを抽出して結合"""
        try:
            soup = BeautifulSoup(html, 'lxml')
            paragraphs = []

            # ヘッダー抽出
            for tag in soup.find_all(['h1', 'h2', 'h3', 'h4', 'h5', 'h6']):
                text = tag.get_text(strip=True)
                if text:
                    paragraphs.append(text)

            # パラグラフ抽出
            for tag in soup.find_all('p'):
                text = tag.get_text(strip=True)
                if text:
                    paragraphs.append(text)

            # コードブロック抽出
            for tag in soup.find_all(['pre', 'code']):
                text = tag.get_text(strip=True)
                if text:
                    paragraphs.append(text)

            # リスト項目抽出
            for tag in soup.find_all('li'):
                text = tag.get_text(strip=True)
                if text:
                    paragraphs.append(text)

            # 構造化コンテンツが見つからない場合
            if not paragraphs:
                for tag in soup.find_all(['div', 'article', 'section']):
                    text = tag.get_text(strip=True)
                    if text and len(text) > 10:
                        paragraphs.append(text)

            if paragraphs:
                return '\n\n'.join(paragraphs)
            else:
                return ArticleTextExtractor._simple_strip_tags(html)

        except Exception:
            return ArticleTextExtractor._simple_strip_tags(html)

    @staticmethod
    def _simple_strip_tags(html: str) -> str:
        """シンプルなタグ除去"""
        soup = BeautifulSoup(html, 'lxml')
        # scriptとstyleを除去
        for tag in soup.find_all(['script', 'style']):
            tag.decompose()
        text = soup.get_text(separator=' ', strip=True)
        # 空白を正規化
        import re
        text = re.sub(r'\s+', ' ', text)
        return text.strip()


class DatabaseManager:
    """データベース管理"""

    def __init__(self, dsn: str):
        self.dsn = dsn
        self.conn: Optional[Connection] = None

    def connect(self):
        """データベースに接続"""
        self.conn = psycopg2.connect(
            self.dsn,
            connect_timeout=10,
            keepalives=1,
            keepalives_idle=30,
            keepalives_interval=10,
            keepalives_count=5
        )
        self.conn.autocommit = False

    def ensure_connection(self):
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
            except Exception:
                pass
            self.connect()

    def close(self):
        """接続を閉じる"""
        if self.conn:
            try:
                self.conn.close()
            except Exception:
                pass
            self.conn = None

    def get_articles_batch(self, offset: int, limit: int) -> List[ArticleRecord]:
        """記事をバッチで取得（HTMLを含むもののみ）"""
        self.ensure_connection()
        cur = self.conn.cursor()
        try:
            # パラメータをタプルで渡す（LIKEパターン、LIMIT, OFFSETの順序）
            cur.execute("""
                SELECT id, title, content, url
                FROM articles
                WHERE content LIKE %s
                ORDER BY id
                LIMIT %s OFFSET %s
            """, ('<%', limit, offset))

            rows = cur.fetchall()
            result = [
                ArticleRecord(
                    id=str(row[0]),
                    title=row[1] or '',
                    content=row[2] or '',
                    url=row[3] or ''
                )
                for row in rows
            ]
            return result
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

    def update_article_batch(self, updates: List[Tuple[str, str]], max_retries: int = 3):
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
                        page_size=100
                    )
                    self.conn.commit()
                    return  # 成功したら終了
                except (psycopg2.OperationalError, psycopg2.InterfaceError) as e:
                    # 接続エラーの場合はロールバックして再接続を試みる
                    try:
                        self.conn.rollback()
                    except Exception:
                        pass
                    if attempt < max_retries - 1:
                        print(f"接続エラーが発生しました。再接続を試みます... (試行 {attempt + 1}/{max_retries})", file=sys.stderr)
                        time.sleep(1)  # 少し待ってから再接続
                        self.conn = None  # 接続を無効化して次回再接続させる
                        continue
                    else:
                        raise  # 最後の試行でも失敗した場合は例外を再発生
                finally:
                    cur.close()
            except Exception as e:
                if attempt == max_retries - 1:
                    raise  # 最後の試行で失敗した場合は例外を再発生
                print(f"バッチ更新エラー: {e}。再試行します... (試行 {attempt + 1}/{max_retries})", file=sys.stderr)
                time.sleep(1)


def process_article(article: ArticleRecord) -> Tuple[str, str, bool]:
    """
    記事を処理してテキスト抽出

    Returns:
        (article_id, extracted_text, success)
    """
    try:
        extracted = ArticleTextExtractor.extract_text(article.content)
        if not extracted or len(extracted.strip()) < 10:
            return (article.id, article.content, False)  # 抽出失敗、元のコンテンツを保持
        return (article.id, extracted, True)
    except Exception as e:
        print(f"Error processing article {article.id}: {e}", file=sys.stderr)
        return (article.id, article.content, False)


def load_env_vars() -> Dict[str, str]:
    """環境変数または.envファイルからデータベース接続情報を読み込む"""
    env_file = Path(__file__).parent.parent / ".env"
    env_vars = {}

    # .envファイルから読み込み
    if env_file.exists():
        with open(env_file, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    env_vars[key.strip()] = value.strip().strip('"').strip("'")

    # 環境変数で上書き
    for key in ["DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_PASSWORD_FILE", "DB_NAME", "POSTGRES_DB"]:
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
        if password_file and os.path.exists(password_file):
            try:
                with open(password_file, "r", encoding="utf-8") as f:
                    db_password = f.read().strip()
            except Exception as e:
                print(f"警告: パスワードファイルの読み込みに失敗しました: {e}", file=sys.stderr)

    if not db_password:
        # 2. secretsディレクトリから読み込みを試行
        script_dir = Path(__file__).parent
        repo_root = script_dir.parent
        secret_path = repo_root / "secrets" / "db_password.txt"
        if secret_path.exists():
            try:
                db_password = secret_path.read_text(encoding="utf-8").strip()
            except Exception as e:
                print(f"警告: secrets/db_password.txtの読み込みに失敗しました: {e}", file=sys.stderr)

    if not db_password:
        print("警告: データベースパスワードが設定されていません。", file=sys.stderr)
        print("以下のいずれかを設定してください:", file=sys.stderr)
        print("  - 環境変数 DB_PASSWORD", file=sys.stderr)
        print("  - 環境変数 DB_PASSWORD_FILE (ファイルパス)", file=sys.stderr)
        print("  - secrets/db_password.txt ファイル", file=sys.stderr)

    return f"postgresql://{db_user}:{db_password}@{db_host}:{db_port}/{db_name}?sslmode=prefer"


def main():
    parser = argparse.ArgumentParser(description='記事HTML→テキスト移行スクリプト')
    parser.add_argument('--batch-size', type=int, default=1000, help='バッチサイズ（デフォルト: 1000）')
    parser.add_argument('--workers', type=int, default=8, help='並列処理数（デフォルト: 8）')
    parser.add_argument('--dry-run', action='store_true', help='ドライラン（実際には更新しない）')
    parser.add_argument('--limit', type=int, help='処理する記事数の上限（テスト用）')
    parser.add_argument('--dsn', type=str, help='PostgreSQL接続文字列（例: postgresql://user:pass@host:port/dbname）')

    args = parser.parse_args()

    # データベース接続文字列の取得
    if args.dsn:
        dsn = args.dsn
    else:
        dsn = get_database_dsn()

    db = DatabaseManager(dsn)

    try:
        print("データベースに接続中...")
        db.connect()

        print("HTMLを含む記事数を取得中...")
        total_count = db.count_html_articles()
        print(f"処理対象記事数: {total_count:,}件")

        if total_count == 0:
            print("処理対象の記事がありません。")
            return

        if args.limit:
            total_count = min(total_count, args.limit)
            print(f"制限により処理数: {total_count:,}件")

        if args.dry_run:
            print("*** ドライランモード: 実際には更新しません ***")

        # 統計情報
        processed = 0
        updated = 0
        failed = 0
        total_original_size = 0
        total_extracted_size = 0

        start_time = time.time()
        last_connection_check = time.time()

        # プログレスバー
        with tqdm(total=total_count, desc="処理中", unit="件") as pbar:
            offset = 0

            while offset < total_count:
                # 定期的に接続を確認（30秒ごと）
                current_time = time.time()
                if current_time - last_connection_check > 30:
                    db.ensure_connection()
                    last_connection_check = current_time

                # バッチ取得
                articles = db.get_articles_batch(offset, args.batch_size)
                if not articles:
                    break

                # 並列処理
                batch_updates = []
                with ThreadPoolExecutor(max_workers=args.workers) as executor:
                    futures = {executor.submit(process_article, article): article for article in articles}

                    for future in as_completed(futures):
                        article_id, extracted_text, success = future.result()
                        article = next(a for f, a in futures.items() if f == future)

                        processed += 1
                        total_original_size += len(article.content)
                        total_extracted_size += len(extracted_text)

                        if success:
                            updated += 1
                            batch_updates.append((article_id, extracted_text))
                        else:
                            failed += 1

                        pbar.update(1)
                        pbar.set_postfix({
                            '更新': updated,
                            '失敗': failed,
                            '削減率': f"{(1 - total_extracted_size / total_original_size) * 100:.1f}%" if total_original_size > 0 else "0%"
                        })

                # バッチ更新
                if batch_updates and not args.dry_run:
                    db.update_article_batch(batch_updates)

                offset += len(articles)

                if args.limit and offset >= args.limit:
                    break

        elapsed_time = time.time() - start_time

        # 結果表示
        print("\n" + "="*60)
        print("移行完了")
        print("="*60)
        print(f"処理記事数: {processed:,}件")
        print(f"更新成功: {updated:,}件")
        print(f"更新失敗: {failed:,}件")
        print(f"処理時間: {elapsed_time:.2f}秒")
        print(f"処理速度: {processed / elapsed_time:.1f}件/秒" if elapsed_time > 0 else "N/A")
        print(f"元のサイズ: {total_original_size / 1024 / 1024:.2f}MB")
        print(f"抽出後サイズ: {total_extracted_size / 1024 / 1024:.2f}MB")
        if total_original_size > 0:
            reduction = (1 - total_extracted_size / total_original_size) * 100
            print(f"削減率: {reduction:.2f}%")

        if args.dry_run:
            print("\n*** ドライランモードでした。実際には更新していません ***")

    except Exception as e:
        print(f"エラーが発生しました: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)

    finally:
        db.close()


if __name__ == '__main__':
    main()

