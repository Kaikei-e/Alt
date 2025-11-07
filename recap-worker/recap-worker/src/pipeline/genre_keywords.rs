/// ジャンル分類用キーワードデータベース。
///
/// 各ジャンルに対して、マルチリンガル（日本語・英語）のキーワードセットを定義します。
use std::collections::HashMap;

/// サポートするジャンルの一覧。
pub(crate) const GENRES: &[&str] = &[
    "ai",
    "tech",
    "business",
    "science",
    "entertainment",
    "sports",
    "politics",
    "health",
    "world",
    "other",
];

/// ジャンル別キーワードマップ。
#[derive(Debug, Clone)]
pub(crate) struct GenreKeywords {
    keywords: HashMap<String, Vec<String>>,
}

impl GenreKeywords {
    /// デフォルトのキーワードマップを構築する。
    #[must_use]
    pub(crate) fn default_keywords() -> Self {
        let mut keywords = HashMap::new();

        // AI - 人工知能、機械学習、自然言語処理、画像認識など
        keywords.insert(
            "ai".to_string(),
            vec![
                // 英語
                "artificial intelligence",
                "machine learning",
                "deep learning",
                "neural network",
                "natural language",
                "computer vision",
                "chatgpt",
                "llm",
                "transformer",
                "reinforcement learning",
                "supervised learning",
                "unsupervised learning",
                "gpt",
                "bert",
                "openai",
                "anthropic",
                "gemini",
                "claude",
                // 日本語
                "人工知能",
                "機械学習",
                "深層学習",
                "ニューラルネット",
                "自然言語処理",
                "画像認識",
                "生成AI",
                "対話型AI",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Tech - 技術全般、ソフトウェア、ハードウェア、クラウドなど
        keywords.insert(
            "tech".to_string(),
            vec![
                // 英語
                "technology",
                "software",
                "hardware",
                "cloud",
                "api",
                "database",
                "kubernetes",
                "docker",
                "microservices",
                "serverless",
                "blockchain",
                "cryptocurrency",
                "cybersecurity",
                "programming",
                "developer",
                "startup",
                "silicon valley",
                "github",
                // 日本語
                "テクノロジー",
                "ソフトウェア",
                "ハードウェア",
                "クラウド",
                "プログラミング",
                "開発者",
                "エンジニア",
                "スタートアップ",
                "セキュリティ",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Business - ビジネス、経済、企業、市場など
        keywords.insert(
            "business".to_string(),
            vec![
                // 英語
                "business",
                "economy",
                "market",
                "stock",
                "finance",
                "investment",
                "startup",
                "revenue",
                "profit",
                "merger",
                "acquisition",
                "ipo",
                "funding",
                "venture capital",
                "ceo",
                "company",
                "corporation",
                "enterprise",
                // 日本語
                "ビジネス",
                "経済",
                "市場",
                "株式",
                "金融",
                "投資",
                "企業",
                "収益",
                "利益",
                "買収",
                "合併",
                "上場",
                "資金調達",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Science - 科学、研究、発見など
        keywords.insert(
            "science".to_string(),
            vec![
                // 英語
                "science",
                "research",
                "study",
                "discovery",
                "experiment",
                "physicist",
                "chemist",
                "biologist",
                "astronomy",
                "physics",
                "chemistry",
                "biology",
                "quantum",
                "genetics",
                "climate",
                "nasa",
                "space",
                "vaccine",
                // 日本語
                "科学",
                "研究",
                "実験",
                "発見",
                "物理学",
                "化学",
                "生物学",
                "天文学",
                "量子",
                "遺伝子",
                "気候",
                "宇宙",
                "ワクチン",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Entertainment - エンターテインメント、映画、音楽、ゲームなど
        keywords.insert(
            "entertainment".to_string(),
            vec![
                // 英語
                "entertainment",
                "movie",
                "film",
                "music",
                "album",
                "concert",
                "game",
                "gaming",
                "streaming",
                "netflix",
                "spotify",
                "youtube",
                "hollywood",
                "celebrity",
                "actor",
                "actress",
                "director",
                "producer",
                // 日本語
                "エンタメ",
                "映画",
                "音楽",
                "アルバム",
                "コンサート",
                "ゲーム",
                "配信",
                "俳優",
                "女優",
                "監督",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Sports - スポーツ全般
        keywords.insert(
            "sports".to_string(),
            vec![
                // 英語
                "sports",
                "football",
                "soccer",
                "basketball",
                "baseball",
                "tennis",
                "olympics",
                "championship",
                "tournament",
                "athlete",
                "player",
                "team",
                "coach",
                "fifa",
                "nba",
                "nfl",
                "mlb",
                "premier league",
                // 日本語
                "スポーツ",
                "サッカー",
                "野球",
                "バスケ",
                "テニス",
                "オリンピック",
                "大会",
                "選手",
                "チーム",
                "監督",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Politics - 政治、政府、選挙など
        keywords.insert(
            "politics".to_string(),
            vec![
                // 英語
                "politics",
                "government",
                "president",
                "minister",
                "election",
                "vote",
                "parliament",
                "congress",
                "senate",
                "policy",
                "legislation",
                "democracy",
                "republican",
                "democrat",
                "brexit",
                "diplomacy",
                "treaty",
                "sanctions",
                // 日本語
                "政治",
                "政府",
                "大統領",
                "首相",
                "選挙",
                "投票",
                "国会",
                "議会",
                "政策",
                "法案",
                "外交",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Health - 健康、医療、病気、治療など
        keywords.insert(
            "health".to_string(),
            vec![
                // 英語
                "health",
                "medical",
                "medicine",
                "disease",
                "treatment",
                "therapy",
                "hospital",
                "doctor",
                "nurse",
                "patient",
                "symptom",
                "diagnosis",
                "vaccine",
                "pandemic",
                "virus",
                "covid",
                "cancer",
                "diabetes",
                // 日本語
                "健康",
                "医療",
                "病気",
                "治療",
                "病院",
                "医師",
                "看護師",
                "患者",
                "症状",
                "診断",
                "ワクチン",
                "パンデミック",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // World - 国際、世界情勢、外交など
        keywords.insert(
            "world".to_string(),
            vec![
                // 英語
                "world",
                "international",
                "global",
                "country",
                "nation",
                "war",
                "conflict",
                "peace",
                "united nations",
                "un",
                "nato",
                "eu",
                "european",
                "asian",
                "african",
                "middle east",
                "refugees",
                "humanitarian",
                // 日本語
                "世界",
                "国際",
                "グローバル",
                "国",
                "戦争",
                "紛争",
                "平和",
                "国連",
                "難民",
                "人道",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        // Other - その他（明確にカテゴライズできないもの）
        keywords.insert(
            "other".to_string(),
            vec![
                // 英語
                "miscellaneous",
                "various",
                "general",
                "other",
                "news",
                // 日本語
                "その他",
                "一般",
                "ニュース",
            ]
            .into_iter()
            .map(String::from)
            .collect(),
        );

        Self { keywords }
    }

    /// 指定されたテキストに対して各ジャンルのスコアを計算する。
    ///
    /// # Returns
    /// ジャンル名をキーとし、マッチしたキーワード数をスコアとするマップ
    #[must_use]
    pub(crate) fn score_text(&self, text: &str) -> HashMap<String, usize> {
        let lowercased = text.to_lowercase();
        let mut scores: HashMap<String, usize> = HashMap::new();

        for (genre, keywords) in &self.keywords {
            let mut score = 0;
            for keyword in keywords {
                if lowercased.contains(&keyword.to_lowercase()) {
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
}
