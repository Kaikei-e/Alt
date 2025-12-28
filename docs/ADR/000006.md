# ネットワークセグメンテーションと動的DNS管理

## ステータス

採択（Accepted）

## コンテキスト

2025年8月上旬、Altプロジェクトはゼロダウンタイムデプロイとサービスメッシュを確立し、本番運用が開始されていた。しかし、セキュリティと運用効率の観点から、以下の課題が顕在化していた：

1. **ネットワークセキュリティの不足**: 全サービスが同一ネットワークセグメント上にあり、侵害時の横展開リスクが高い
2. **DNS解決の非効率性**: 外部DNSクエリが頻発し、レイテンシとコストが増加
3. **テーマの柔軟性不足**: Vaporwaveテーマのみで、ユーザーの好みに応じた選択肢がない
4. **フィード登録の脆弱性**: 無効なURLやタイムアウトするフィードが登録され、システムに負荷をかける

特に、Kubernetesのデフォルトネットワークポリシー（全Podが相互通信可能）は、ゼロトラストセキュリティの観点から不十分であり、明示的なネットワーク分離が必要とされていた。

## 決定

セキュリティ、パフォーマンス、ユーザー体験を向上させるため、以下の改善を実施した：

### 1. Kubernetesネットワークポリシーによるゼロトラスト

**ゼロトラストの原則:**
- デフォルトで全ての通信を拒否
- 必要最小限の通信のみ許可
- マイクロセグメンテーションによる横展開の防止

**ネームスペース分離:**
```
alt-apps:        フロントエンド、バックエンドAPI
alt-processing:  Pre-processor、News-creator、Tag-generator
alt-database:    PostgreSQL、Meilisearch、ClickHouse
alt-auth:        Kratos、Auth-hub、Auth-token-manager
```

**ネットワークポリシー例:**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-backend-to-db
  namespace: alt-database
spec:
  podSelector:
    matchLabels:
      app: postgresql
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: alt-apps
          podSelector:
            matchLabels:
              app: alt-backend
```

**ポリシー設計:**
- **alt-apps → alt-database**: バックエンドからデータベースへのアクセスのみ許可
- **alt-processing → alt-database**: 処理サービスからデータベースへのアクセスのみ許可
- **alt-apps → alt-auth**: 認証フローのみ許可
- **インターネット → alt-apps**: Ingress経由のHTTP/HTTPSのみ許可

**効果:**
- 攻撃者が1つのサービスを侵害しても、他のサービスへアクセス不可
- 最小権限の原則に基づいた通信制御
- コンプライアンス要件への適合

### 2. 動的DNSリゾルバ（fsnotifyベース）

**課題:**
- 外部DNS解決のレイテンシ（特にDocker内）
- DNS解決失敗時のリトライコスト
- 頻繁に変更されるドメインへの対応

**解決策:**
```go
type DynamicDNSResolver struct {
    Cache       map[string]net.IP
    Watcher     *fsnotify.Watcher
    ConfigFile  string // /etc/hosts or custom file
}

func (d *DynamicDNSResolver) Watch() {
    for {
        select {
        case event := <-d.Watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                d.Reload()
            }
        }
    }
}
```

**機能:**
- **オンメモリキャッシュ**: DNS解決結果をメモリに保持
- **ファイルベース管理**: `/etc/hosts`風のファイルでドメインとIPを管理
- **自動リロード**: fsnotifyでファイル変更を検知し、キャッシュを自動更新
- **フォールバック**: キャッシュにない場合は外部DNSへフォールバック

**効果:**
- DNS解決レイテンシがほぼゼロ（メモリアクセス）
- 外部DNSクエリの大幅削減
- 動的なドメイン管理（Kubernetesサービスの追加/削除に対応）

### 3. Alt-Paperテーマシステム

**背景:**
- Vaporwaveテーマは一部のユーザーには派手すぎる
- ビジネス用途やアクセシビリティ重視のユーザーへの対応が必要

**Alt-Paperテーマ:**
- **コンセプト**: ミニマリズムとクリーンデザイン
- **特徴**: 白を基調、シンプルなタイポグラフィ、控えめなアクセントカラー
- **アクセシビリティ**: WCAG 2.1 AAコンプライアンス

**実装:**
```typescript
const themes = {
  vaporwave: {
    colors: {
      primary: '#FF6AC1',
      background: 'rgba(10, 10, 35, 0.8)',
      glass: 'rgba(255, 255, 255, 0.1)',
    }
  },
  paper: {
    colors: {
      primary: '#2563EB',
      background: '#FFFFFF',
      surface: '#F9FAFB',
    }
  }
};
```

**テーマ管理:**
- ユーザー設定でテーマ切り替え
- LocalStorageでテーマ設定を永続化
- CSS変数による動的テーマ適用

### 4. フィード登録バリデーション強化

**脆弱性:**
- 無効なURL（`htp://`など）が登録される
- タイムアウトするフィードがシステムに負荷をかける
- セキュリティリスク（SSRFの可能性）

**改善:**
```go
type FeedValidator struct {
    Timeout        time.Duration // 10秒
    AllowedSchemes []string      // ["http", "https"]
    DenyList       []string      // 禁止ドメイン
}

func (v *FeedValidator) Validate(url string) error {
    // 1. URL形式の検証
    // 2. スキームの検証
    // 3. Denylistチェック
    // 4. タイムアウト付きHTTPリクエスト
    // 5. Content-Typeの検証（RSS/Atom）
}
```

**バリデーション項目:**
1. **URL形式**: Go の `net/url` パッケージで解析
2. **スキーム**: http/httpsのみ許可
3. **Denylist**: プライベートIPアドレス、ローカルホストを拒否
4. **タイムアウト**: 10秒以内に応答がない場合は拒否
5. **Content-Type**: `application/rss+xml`、`application/atom+xml`を確認
6. **セキュリティチェック**: XMLボム攻撃の検出

**ログ記録:**
- バリデーション失敗時に詳細なエラーログ
- 不正なURL登録試行の監視
- 統計情報の収集（成功率、平均応答時間）

## 結果・影響

### 利点

1. **セキュリティの大幅強化**
   - ゼロトラストネットワークにより、横展開攻撃を防止
   - ネームスペース分離で攻撃範囲を限定
   - フィード登録バリデーションでSSRF攻撃を防御

2. **パフォーマンス向上**
   - 動的DNSリゾルバでDNS解決レイテンシをほぼゼロ化
   - 外部DNSクエリの削減によりコスト削減
   - 無効なフィード登録の防止でシステム負荷軽減

3. **ユーザー体験の向上**
   - Alt-Paperテーマで幅広いユーザー層に対応
   - アクセシビリティ向上
   - フィード登録の成功率向上（無効なURLの排除）

4. **運用効率の改善**
   - ネットワークポリシーによるセキュリティの自動化
   - DNS管理の自動化（fsnotify）
   - フィードバリデーションによる問題の早期発見

### 注意点・トレードオフ

1. **ネットワークポリシーの管理コスト**
   - 新サービス追加時にポリシー更新が必要
   - デバッグ時の通信トラブルシューティング
   - ポリシー設定ミスによる意図しない通信遮断

2. **動的DNSリゾルバの制約**
   - ファイルベース管理は小規模環境向き（大規模には不向き）
   - 外部DNSの変更をリアルタイムで反映できない
   - フォールバックのレイテンシ

3. **テーマシステムのメンテナンス**
   - 複数テーマの保守コスト
   - テーマ間の一貫性維持
   - コンポーネントのテーマ対応

4. **フィードバリデーションの厳格性**
   - 一部の合法的なフィードが拒否される可能性
   - タイムアウト設定が短すぎると、遅いサーバーのフィードが拒否される

## 参考コミット

- `79971f76` - Implement Kubernetes network policies for namespaces
- `3ffbec37` - Configure network segmentation for alt-apps, alt-processing, alt-database, alt-auth
- `05657518` - Update skaffold with network policies
- `7f4497dd` - Implement dynamic DNS resolver with fsnotify
- `60d78e5f` - Add fsnotify dependency for file watching
- `ba99b75b` - Introduce alt-paper theme
- `141762c8` - Update theme management system for multiple themes
- `7b4d9665` - Add theme color definitions for alt-paper
- `c33bbe4c` - Enhance feed registration logic with validation
- `20b32626` - Add timeout and format validation for feed URLs
- `9788f6b1` - Add detailed logging for feed validation
