/// ジャンル分類用キーワードデータベース。
///
/// 各ジャンルに対して、マルチリンガル（日本語・英語）のキーワードセットを定義します。
use std::collections::HashMap;

/// ジャンル別キーワードマップ。
#[derive(Debug, Clone)]
pub(crate) struct GenreKeywords {
    keywords: HashMap<String, Vec<String>>,
    negative_keywords: HashMap<String, Vec<String>>,
}

impl GenreKeywords {
    /// デフォルトのキーワードマップを構築する。
    #[must_use]
    #[allow(clippy::too_many_lines, clippy::pedantic)]
    pub(crate) fn default_keywords() -> Self {
        let mut keywords = HashMap::new();
        let mut negative_keywords = HashMap::new();

        fn push(genres: &mut HashMap<String, Vec<String>>, name: &str, words: &[&str]) {
            genres.insert(
                name.to_string(),
                words
                    .iter()
                    .map(|w| w.trim().to_lowercase())
                    .filter(|w| !w.is_empty())
                    .collect(),
            );
        }

        fn push_negative(genres: &mut HashMap<String, Vec<String>>, name: &str, words: &[&str]) {
            genres.insert(
                name.to_string(),
                words
                    .iter()
                    .map(|w| w.trim().to_lowercase())
                    .filter(|w| !w.is_empty())
                    .collect(),
            );
        }

        push(
            &mut keywords,
            "ai",
            &[
                "artificial intelligence",
                "ai",
                "machine learning",
                "ml",
                "deep learning",
                "neural network",
                "foundation model",
                "large language model",
                "llm",
                "generative ai",
                "ai safety",
                "alignment",
                "rlhf",
                "vector database",
                "inference",
                "gpu cluster",
                "chiplet",
                "openai",
                "chatgpt",
                "anthropic",
                "gemini",
                "claude",
                "mistral",
                "huggingface",
                "transformer",
                "diffusion model",
                "self-supervised",
                "few-shot",
                "zero-shot",
                "artificial general intelligence",
                "人工知能",
                "機械学習",
                "深層学習",
                "ニューラルネット",
                "大規模言語モデル",
                "生成ai",
                "強化学習",
                "ai倫理",
                "ai安全性",
                "aiガバナンス",
                "ベクトルデータベース",
                "gpuクラスタ",
                "量子ai",
            ],
        );

        push(
            &mut keywords,
            "art_culture",
            &[
                "art",
                "arts",
                "artistic",
                "gallery",
                "museum",
                "exhibition",
                "biennale",
                "installation",
                "digital art",
                "generative art",
                "nft art",
                "curator",
                "critique",
                "philosophy",
                "aesthetics",
                "heritage",
                "literature",
                "essay",
                "poetry",
                "theatre",
                "performing arts",
                "calligraphy",
                "tea ceremony",
                "craft",
                "アート",
                "美術",
                "芸術祭",
                "展覧会",
                "伝統工芸",
                "文化財",
                "哲学",
                "美学",
                "芸術評論",
            ],
        );

        push(
            &mut keywords,
            "developer_insights",
            &[
                "architecture review",
                "system design",
                "scalability",
                "resilience",
                "fault tolerance",
                "postmortem",
                "incident review",
                "root cause analysis",
                "benchmark",
                "latency",
                "throughput",
                "performance tuning",
                "observability",
                "profiling",
                "debugging",
                "technical debt",
                "refactoring",
                "monolith",
                "microservice",
                "service mesh",
                "api design",
                "sdk",
                "developer experience",
                "engineering blog",
                "モノレポ",
                "アーキテクチャ",
                "設計思想",
                "技術検証",
                "技術ブログ",
                "負荷試験",
                "可観測性",
                "技術選定",
            ],
        );

        push(
            &mut keywords,
            "pro_it_media",
            &[
                "enterprise it",
                "it management",
                "itil",
                "sla",
                "sase",
                "ztna",
                "network architecture",
                "datacenter",
                "colocation",
                "hybrid cloud",
                "observability platform",
                "service desk",
                "it operations",
                "policy engine",
                "compliance",
                "governance",
                "vendor",
                "cio",
                "cto",
                "システム運用",
                "大企業it",
                "it統制",
                "情報システム部",
                "社内it",
                "ゼロトラスト",
                "セキュリティガバナンス",
            ],
        );

        push(
            &mut keywords,
            "consumer_tech",
            &[
                "consumer tech",
                "gadget",
                "smartphone",
                "iphone",
                "android",
                "pixel",
                "galaxy",
                "xiaomi",
                "tablet",
                "laptop",
                "notebook pc",
                "wearable",
                "smartwatch",
                "earbuds",
                "ar glasses",
                "vr headset",
                "homepod",
                "smart home",
                "review",
                "hands-on",
                "leak",
                "rumor",
                "デバイス",
                "ガジェット",
                "家電",
                "レビュー",
                "リーク情報",
                "比較記事",
            ],
        );

        push(
            &mut keywords,
            "global_politics",
            &[
                "geopolitics",
                "global politics",
                "foreign policy",
                "sanctions",
                "trade war",
                "security alliance",
                "strategic competition",
                "multipolar",
                "global south",
                "regional bloc",
                "summit",
                "defense pact",
                "アフリカ外交",
                "インド太平洋",
                "国際秩序",
                "覇権",
                "勢力圏",
                "地政学",
                "外交戦略",
                "国際安全保障",
            ],
        );

        push(
            &mut keywords,
            "environment_policy",
            &[
                "net zero policy",
                "carbon tax",
                "emissions trading",
                "climate bill",
                "paris agreement",
                "cop28",
                "cop29",
                "climate pact",
                "just transition",
                "green finance",
                "renewable mandate",
                "energy transition",
                "circular economy act",
                "脱炭素政策",
                "排出量取引",
                "再生可能エネルギー義務",
                "気候法案",
                "グリーン成長",
                "ゼロエミッション都市",
            ],
        );

        push(
            &mut keywords,
            "society_justice",
            &[
                "social justice",
                "human rights",
                "civil rights",
                "civil liberties",
                "inequality",
                "equity",
                "diversity",
                "inclusion",
                "belonging",
                "gender equality",
                "lgbtq",
                "anti-racism",
                "labor rights",
                "union",
                "strike",
                "public safety",
                "gun control",
                "housing justice",
                "education reform",
                "social impact",
                "社会課題",
                "人権",
                "平等",
                "差別",
                "多様性",
                "包摂",
                "ジェンダー",
                "市民権",
                "労働争議",
                "司法改革",
            ],
        );

        push(
            &mut keywords,
            "travel_lifestyle",
            &[
                "travel",
                "tourism",
                "itinerary",
                "flight deal",
                "airline",
                "airport",
                "visa",
                "immigration",
                "hotel",
                "resort",
                "boutique hotel",
                "hostel",
                "camping",
                "glamping",
                "road trip",
                "city guide",
                "hidden spot",
                "food tour",
                "cultural experience",
                "観光",
                "旅行記",
                "旅程",
                "おすすめスポット",
                "ホテルレビュー",
                "世界遺産",
                "観光庁",
                "旅のアイデア",
            ],
        );

        push(
            &mut keywords,
            "security_policy",
            &[
                "security policy",
                "cybersecurity policy",
                "compliance",
                "risk management",
                "data residency",
                "privacy regulation",
                "gdpr",
                "ccpa",
                "hipaa",
                "iso 27001",
                "soc2",
                "fedramp",
                "nist",
                "zero trust architecture",
                "governance",
                "security framework",
                "個人情報保護法",
                "情報ガバナンス",
                "セキュリティ基準",
                "監査",
                "内部統制",
                "セキュリティポリシー",
            ],
        );

        push(
            &mut keywords,
            "business_finance",
            &[
                "finance",
                "financial",
                "banking",
                "capital market",
                "treasury",
                "bond",
                "interest rate",
                "monetary policy",
                "fiscal policy",
                "hedge fund",
                "asset management",
                "wealth management",
                "earnings per share",
                "eps",
                "cash flow",
                "balance sheet",
                "valuation",
                "macro",
                "micro",
                "hedging",
                "derivatives",
                "crypto",
                "fintech",
                "金融",
                "銀行",
                "証券",
                "金利",
                "マクロ経済",
                "財務分析",
                "金融市場",
            ],
        );

        push(
            &mut keywords,
            "ai_research",
            &[
                "ai research",
                "research paper",
                "preprint",
                "arxiv",
                "neurips",
                "iclr",
                "icml",
                "cvpr",
                "acl",
                "emnlp",
                "siggraph",
                "nips",
                "icra",
                "longformer",
                "transformer",
                "diffusion",
                "graph neural network",
                "self-supervised learning",
                "few-shot learning",
                "research lab",
                "研究論文",
                "プレプリント",
                "学会",
                "ai研究",
                "学術的",
                "研究成果",
            ],
        );

        push(
            &mut keywords,
            "ai_policy",
            &[
                "ai policy",
                "ai regulation",
                "ai governance",
                "ai act",
                "nista i rmf",
                "responsible ai",
                "responsible innovation",
                "alignment",
                "safety board",
                "ai audit",
                "model card",
                "data governance",
                "ethics board",
                "policy sandbox",
                "ai oversight",
                "ai責任",
                "ai法規制",
                "aiガバナンス",
                "ai倫理",
                "ai安全性",
                "ai監査",
                "モデル開示",
            ],
        );

        push(
            &mut keywords,
            "games_puzzles",
            &[
                "game",
                "gaming",
                "videogame",
                "mobile game",
                "pc gaming",
                "console",
                "playstation",
                "xbox",
                "nintendo",
                "switch",
                "steam",
                "esports",
                "tournament",
                "speedrun",
                "patch notes",
                "update",
                "dlc",
                "loot box",
                "gacha",
                "crossword",
                "sudoku",
                "puzzle",
                "boardgame",
                "tabletop",
                "ttrpg",
                "カードゲーム",
                "攻略",
                "ゲームレビュー",
                "アップデート情報",
                "eスポーツ",
            ],
        );

        push(
            &mut keywords,
            "other",
            &[
                "misc",
                "various",
                "general",
                "digest",
                "news roundup",
                "topics",
                "ハイライト",
                "ざっくり",
                "まとめ",
                "その他",
            ],
        );

        push(
            &mut keywords,
            "tech",
            &[
                "technology",
                "software",
                "hardware",
                "cloud",
                "multicloud",
                "saas",
                "paas",
                "serverless",
                "microservices",
                "monorepo",
                "kubernetes",
                "docker",
                "devops",
                "observability",
                "sre",
                "edge computing",
                "semiconductor",
                "foundry",
                "chip design",
                "risc-v",
                "5g",
                "6g",
                "telecom",
                "quantum computing",
                "startup",
                "scaleup",
                "programming",
                "developer",
                "engineer",
                "code",
                "application",
                "system development",
                "api",
                "sdk",
                "framework",
                "library",
                "repository",
                "version control",
                "git",
                "ci/cd",
                "infrastructure",
                "システム開発",
                "テック企業",
                "スタートアップ",
                "半導体",
                "クラウド",
                "デジタル化",
                "dx",
                "iot",
                "基幹システム",
                "itモダナイゼーション",
                "プログラミング",
                "ソフトウェア",
                "開発者",
                "エンジニア",
                "コード",
                "アプリケーション",
                "システム開発",
                "技術",
                "it",
                "インフラ",
            ],
        );

        push(
            &mut keywords,
            "business",
            &[
                "business",
                "economy",
                "macro",
                "earnings",
                "earning call",
                "guidance",
                "ipo",
                "spac",
                "revenue",
                "profit",
                "loss",
                "dividend",
                "valuation",
                "funding",
                "venture capital",
                "private equity",
                "merger",
                "acquisition",
                "takeover",
                "recession",
                "inflation",
                "supply chain",
                "interest rate",
                "bond yield",
                "stock market",
                "nasdaq",
                "dow jones",
                "経済",
                "景気",
                "金融政策",
                "企業買収",
                "決算",
                "資金調達",
                "上場",
                "株価",
                "日経平均",
                "財務",
                "ビジネスモデル",
            ],
        );

        push(
            &mut keywords,
            "politics",
            &[
                "politics",
                "government",
                "administration",
                "cabinet",
                "president",
                "prime minister",
                "election",
                "campaign",
                "ballot",
                "manifesto",
                "policy",
                "bill",
                "legislation",
                "parliament",
                "congress",
                "senate",
                "house of representatives",
                "diplomacy",
                "summit",
                "treaty",
                "sanctions",
                "geopolitics",
                "coalition",
                "referendum",
                "政権",
                "政治",
                "国会",
                "議会",
                "法案",
                "首相",
                "大統領",
                "選挙",
                "党首",
                "公約",
                "外交",
                "安全保障",
            ],
        );

        push(
            &mut keywords,
            "health",
            &[
                "health",
                "healthcare",
                "medical",
                "medicine",
                "hospital",
                "clinic",
                "doctor",
                "nurse",
                "patient",
                "disease",
                "pandemic",
                "epidemic",
                "outbreak",
                "vaccination",
                "vaccine",
                "public health",
                "telemedicine",
                "digital health",
                "biotech",
                "pharma",
                "medtech",
                "mRNA",
                "genomics",
                "臨床試験",
                "免疫",
                "感染症",
                "治療",
                "診断",
                "医療制度",
                "医薬品",
                "ワクチン",
                "公衆衛生",
            ],
        );

        push(
            &mut keywords,
            "sports",
            &[
                "sport",
                "sports",
                "football",
                "soccer",
                "baseball",
                "basketball",
                "rugby",
                "tennis",
                "golf",
                "athletics",
                "marathon",
                "world cup",
                "fifa",
                "nba",
                "nfl",
                "mlb",
                "euroleague",
                "olympics",
                "tournament",
                "playoffs",
                "transfer window",
                "draft",
                "coach",
                "athlete",
                "stadium",
                "サッカー",
                "野球",
                "バスケ",
                "テニス",
                "オリンピック",
                "スポーツ",
                "選手",
                "監督",
                "リーグ",
                "プロ野球",
                "jリーグ",
                "wbc",
            ],
        );

        push(
            &mut keywords,
            "science",
            &[
                "science",
                "research",
                "study",
                "academic",
                "paper",
                "peer reviewed",
                "laboratory",
                "experiment",
                "physics",
                "chemistry",
                "biology",
                "astronomy",
                "astrophysics",
                "genomics",
                "crispr",
                "space",
                "rocket",
                "nasa",
                "jaxa",
                "esa",
                "quantum",
                "particle",
                "telescope",
                "climate science",
                "geology",
                "oceanography",
                "論文",
                "研究",
                "科学",
                "実験",
                "天文学",
                "宇宙",
                "遺伝子",
                "量子",
                "観測",
            ],
        );

        push(
            &mut keywords,
            "entertainment",
            &[
                "entertainment",
                "movie",
                "film",
                "cinema",
                "box office",
                "hollywood",
                "bollywood",
                "anime",
                "manga",
                "k-drama",
                "series",
                "streaming",
                "netflix",
                "disney+",
                "prime video",
                "spotify",
                "music",
                "album",
                "concert",
                "tour",
                "celebrity",
                "idol",
                "red carpet",
                "award",
                "オタク文化",
                "エンタメ",
                "映画",
                "音楽",
                "アニメ",
                "マンガ",
                "ライブ",
                "俳優",
                "芸能",
                "配信サービス",
            ],
        );

        push(
            &mut keywords,
            "world",
            &[
                "world",
                "international",
                "global",
                "geopolitics",
                "multilateral",
                "alliance",
                "summit",
                "foreign minister",
                "foreign policy",
                "trade dispute",
                "conflict",
                "ceasefire",
                "refugee",
                "humanitarian",
                "united nations",
                "security council",
                "nato",
                "g7",
                "g20",
                "asean",
                "africa",
                "middle east",
                "latin america",
                "アジア太平洋",
                "国際情勢",
                "世界",
                "外交",
                "国連",
                "難民問題",
                "多極化",
            ],
        );

        push(
            &mut keywords,
            "security",
            &[
                "security",
                "cybersecurity",
                "infosec",
                "zero trust",
                "sase",
                "ztna",
                "siem",
                "soar",
                "endpoint",
                "xdr",
                "ids",
                "ips",
                "threat intelligence",
                "ransomware",
                "malware",
                "spyware",
                "phishing",
                "social engineering",
                "data breach",
                "vulnerability",
                "cve",
                "patch",
                "bug bounty",
                "ciso",
                "soc team",
                "ゼロトラスト",
                "サイバー攻撃",
                "情報漏洩",
                "脆弱性",
                "マルウェア",
                "侵入検知",
                "セキュリティ運用",
            ],
        );

        push(
            &mut keywords,
            "product",
            &[
                "product",
                "product management",
                "product strategy",
                "roadmap",
                "backlog",
                "feature",
                "launch",
                "release notes",
                "rollout",
                "mvp",
                "product-market fit",
                "customer interview",
                "user research",
                "persona",
                "journey map",
                "kpi",
                "nps",
                "engagement",
                "retention",
                "a/b test",
                "cohort",
                "ユーザー価値",
                "プロダクトマネージャー",
                "pdｍ",
                "仮説検証",
                "仕様書",
                "機能改善",
                "プロダクト戦略",
            ],
        );

        push(
            &mut keywords,
            "design",
            &[
                "design",
                "ux",
                "ui",
                "service design",
                "interaction design",
                "information architecture",
                "design system",
                "figma",
                "sketch",
                "adobe xd",
                "framer",
                "prototype",
                "wireframe",
                "mockup",
                "visual design",
                "branding",
                "typography",
                "color palette",
                "layout",
                "motion design",
                "accessibility",
                "inclusive design",
                "デザイン",
                "ユーザー体験",
                "インターフェース",
                "スタイルガイド",
                "モックアップ",
                "アクセシビリティ",
            ],
        );

        push(
            &mut keywords,
            "culture",
            &[
                "culture",
                "cultural",
                "heritage",
                "tradition",
                "festival",
                "ritual",
                "anthropology",
                "sociology",
                "humanities",
                "philosophy",
                "literature",
                "poetry",
                "history",
                "museum",
                "gallery",
                "religion",
                "spirituality",
                "folklore",
                "人文科学",
                "文化",
                "文化的",
                "伝統芸能",
                "博物館",
                "祭り",
                "思想",
                "社会学",
                "宗教",
            ],
        );

        push(
            &mut keywords,
            "environment",
            &[
                "environment",
                "climate",
                "global warming",
                "climate change",
                "decarbonization",
                "carbon footprint",
                "carbon neutral",
                "net zero",
                "renewable energy",
                "solar",
                "wind power",
                "hydrogen",
                "battery",
                "ev",
                "sustainability",
                "biodiversity",
                "ecosystem",
                "conservation",
                "recycling",
                "circular economy",
                "esg",
                "sdgs",
                "気候変動",
                "脱炭素",
                "再生可能エネルギー",
                "環境保護",
                "生物多様性",
                "温暖化対策",
            ],
        );

        push(
            &mut keywords,
            "lifestyle",
            &[
                "lifestyle",
                "life",
                "wellness",
                "fitness",
                "exercise",
                "yoga",
                "mindfulness",
                "meditation",
                "nutrition",
                "diet",
                "vegan",
                "home decor",
                "minimalism",
                "travel hack",
                "weekend getaway",
                "fashion",
                "beauty",
                "cosmetics",
                "skincare",
                "pet-friendly",
                "remote work",
                "digital nomad",
                "暮らし",
                "ライフスタイル",
                "美容",
                "健康",
                "旅行",
                "インテリア",
                "カフェ",
                "趣味",
            ],
        );

        push(
            &mut keywords,
            "other",
            &[
                "misc",
                "general",
                "uncategorized",
                "その他",
                "一般",
                "未分類",
            ],
        );

        // Negative Keywords (Noise Reduction)
        // Politics: Exclude product reviews, deals, and tech gadgets that often get misclassified due to "policy" or "law" terms.
        push_negative(
            &mut negative_keywords,
            "politics",
            &[
                "apple watch",
                "kindle",
                "deal",
                "sale",
                "discount",
                "coupon",
                "review",
                "best buy",
                "amazon",
                "black friday",
                "cyber monday",
                "promo code",
                "gift card",
                "shopping",
                "bargain",
                "clearance",
                "セール",
                "クーポン",
                "割引",
                "レビュー",
                "アマゾン",
                "お買い得",
                "最安値",
            ],
        );

        // Business: Exclude art/culture events and "lifehack" style money tips.
        push_negative(
            &mut negative_keywords,
            "business",
            &[
                "art exhibition",
                "museum",
                "gallery",
                "painting",
                "sculpture",
                "concert",
                "festival",
                "株主優待",
                "ポイ活",
                "節約術",
                "展覧会",
                "美術館",
                "コンサート",
                "フェス",
            ],
        );

        Self {
            keywords,
            negative_keywords,
        }
    }

    /// テキストに含まれるキーワードに基づいてジャンルごとのスコアを計算する。
    ///
    /// # Arguments
    /// * `text` - 分析対象のテキスト
    ///
    /// # Returns
    /// ジャンル名をキー、出現回数（スコア）を値とするマップ
    #[must_use]
    pub(crate) fn score_text(&self, text: &str) -> HashMap<String, usize> {
        let text_lower = text.to_lowercase();
        let mut scores = HashMap::new();

        for (genre, words) in &self.keywords {
            // Check negative keywords first
            if let Some(negatives) = self.negative_keywords.get(genre) {
                let has_negative = negatives.iter().any(|neg| text_lower.contains(neg));
                if has_negative {
                    continue;
                }
            }

            let mut score = 0;
            for word in words {
                if word.is_ascii() {
                    // For ASCII keywords, enforce word boundaries
                    let mut found = false;
                    for (start, _) in text_lower.match_indices(word) {
                        let bytes = text_lower.as_bytes();
                        let boundary_before = if start == 0 {
                            true
                        } else {
                            !bytes[start - 1].is_ascii_alphanumeric()
                        };

                        let end = start + word.len();
                        let boundary_after = if end >= bytes.len() {
                            true
                        } else {
                            !bytes[end].is_ascii_alphanumeric()
                        };

                        if boundary_before && boundary_after {
                            found = true;
                            break;
                        }
                    }
                    if found {
                        score += 1;
                    }
                } else if text_lower.contains(word) {
                    score += 1;
                }
            }
            if score > 0 {
                scores.insert(genre.clone(), score);
            }
        }

        scores
    }

    /// テキストに最もマッチするジャンルを最大N個返す。
    ///
    /// # Arguments
    /// * `text` - スコアリングするテキスト
    /// * `max_genres` - 返す最大ジャンル数
    ///
    /// # Returns
    /// (genre, score) のタプルをスコアの降順で返す
    #[must_use]
    pub(crate) fn top_genres(&self, text: &str, max_genres: usize) -> Vec<(String, usize)> {
        let mut scores: Vec<(String, usize)> = self.score_text(text).into_iter().collect();

        // スコアの降順でソート
        scores.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));

        // 最大N個を返す
        scores.into_iter().take(max_genres).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn score_text_matches_keywords() {
        let keywords = GenreKeywords::default_keywords();
        let text =
            "This article discusses artificial intelligence and machine learning in healthcare.";
        let scores = keywords.score_text(text);

        assert!(scores.contains_key("ai"));
        assert!(scores.contains_key("health"));
        assert!(scores["ai"] > 0);
    }

    #[test]
    fn score_text_handles_japanese() {
        let keywords = GenreKeywords::default_keywords();
        let text = "この記事では人工知能と機械学習について説明します。";
        let scores = keywords.score_text(text);

        assert!(scores.contains_key("ai"));
        assert!(scores["ai"] > 0);
    }

    #[test]
    fn top_genres_returns_sorted() {
        let keywords = GenreKeywords::default_keywords();
        let text = "AI and machine learning are revolutionizing healthcare and medical research.";
        let top = keywords.top_genres(text, 3);

        assert!(!top.is_empty());
        // 最初が最もスコアの高いジャンル
        assert!(top[0].1 >= top.get(1).map_or(0, |t| t.1));
    }

    #[test]
    fn top_genres_limits_results() {
        let keywords = GenreKeywords::default_keywords();
        let text = "Technology, science, health, and business news from around the world.";
        let top = keywords.top_genres(text, 2);

        assert!(top.len() <= 2);
    }

    #[test]
    fn negative_keywords_exclude_genre() {
        let keywords = GenreKeywords::default_keywords();
        // "policy" might trigger politics, but "Apple Watch" and "deal" should trigger negative keywords for politics.
        let text = "New Apple Watch deal! Check out the privacy policy update.";
        let scores = keywords.score_text(text);

        // Politics should be excluded despite "policy" being present
        assert!(!scores.contains_key("politics"));
    }

    #[test]
    fn negative_keywords_business_exclusion() {
        let keywords = GenreKeywords::default_keywords();
        // "investment" might trigger business, but "art exhibition" should exclude it.
        let text = "The new art exhibition is a great investment of your time.";
        let scores = keywords.score_text(text);

        assert!(!scores.contains_key("business"));
    }
}
