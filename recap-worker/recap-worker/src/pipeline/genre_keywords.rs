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
        let mut m = HashMap::new();

        // Helper to insert keywords
        let mut insert = |id: &str, positive: &[&str], negative: &[&str]| {
            let pos: Vec<String> = positive.iter().map(|s| s.to_string()).collect();
            let neg: Vec<String> = negative.iter().map(|s| s.to_string()).collect();
            m.insert(id.to_string(), (pos, neg));
        };

        insert(
            "ai_data",
            &[
                "ai",
                "machine learning",
                "data",
                "analytics",
                "generative",
                "llm",
                "neural network",
                "big data",
                "人工知能",
                "機械学習",
                "データ",
            ],
            &[],
        );
        insert(
            "software_dev",
            &[
                "software",
                "programming",
                "developer",
                "code",
                "devops",
                "cloud",
                "framework",
                "api",
                "git",
                "ci/cd",
                "ソフトウェア",
                "プログラミング",
                "開発",
            ],
            &["game", "esports"],
        );
        insert(
            "cybersecurity",
            &[
                "security",
                "cyber",
                "vulnerability",
                "hack",
                "malware",
                "ransomware",
                "phishing",
                "encryption",
                "セキュリティ",
                "サイバー",
                "脆弱性",
            ],
            &[],
        );
        insert(
            "consumer_tech",
            &[
                "smartphone",
                "gadget",
                "pc",
                "wearable",
                "electronics",
                "device",
                "apple",
                "android",
                "iphone",
                "スマホ",
                "ガジェット",
                "家電",
            ],
            &["industry", "manufacturing"],
        );
        insert(
            "internet_platforms",
            &[
                "social media",
                "sns",
                "platform",
                "search engine",
                "streaming",
                "app store",
                "google",
                "twitter",
                "facebook",
                "youtube",
                "プラットフォーム",
                "検索",
            ],
            &[],
        );
        insert(
            "space_astronomy",
            &[
                "space",
                "astronomy",
                "nasa",
                "satellite",
                "rocket",
                "planet",
                "galaxy",
                "orbit",
                "star",
                "宇宙",
                "天文学",
                "衛星",
            ],
            &[],
        );
        insert(
            "climate_environment",
            &[
                "climate change",
                "environment",
                "carbon",
                "emissions",
                "global warming",
                "pollution",
                "sustainability",
                "気候変動",
                "環境",
                "温暖化",
            ],
            &["investing", "market"],
        );
        insert(
            "energy_transition",
            &[
                "energy",
                "renewable",
                "solar",
                "wind",
                "battery",
                "nuclear",
                "power",
                "grid",
                "transition",
                "エネルギー",
                "再生可能",
                "電力",
            ],
            &[],
        );
        insert(
            "health_medicine",
            &[
                "health",
                "healthcare",
                "medicine",
                "hospital",
                "doctor",
                "disease",
                "vaccine",
                "treatment",
                "medical",
                "医療",
                "健康",
                "病院",
                "治療",
                "ヘルスケア",
            ],
            &["fitness", "yoga"], // maybe distinct from lifestyle
        );
        insert(
            "life_science",
            &[
                "biology",
                "genetics",
                "dna",
                "biotech",
                "research",
                "cell",
                "biodiversity",
                "生命科学",
                "生物学",
                "遺伝子",
            ],
            &[],
        );
        insert(
            "economics_macro",
            &[
                "economy",
                "inflation",
                "gdp",
                "recession",
                "interest rate",
                "unemployment",
                "currency",
                "trade",
                "経済",
                "インフレ",
                "景気",
            ],
            &["stock", "market"], // micro finance/markets separated
        );
        insert(
            "markets_finance",
            &[
                "market",
                "finance",
                "stock",
                "bond",
                "investment",
                "bank",
                "earnings",
                "wall street",
                "金融",
                "市場",
                "株",
                "投資",
            ],
            &["art", "exhibition"],
        );
        insert(
            "startups_innovation",
            &[
                "startup",
                "innovation",
                "venture capital",
                "fundraising",
                "founder",
                "entrepreneur",
                "unicorn",
                "スタートアップ",
                "ベンチャー",
                "イノベーション",
            ],
            &[],
        );
        insert(
            "industry_logistics",
            &[
                "industry",
                "logistics",
                "manufacturing",
                "supply chain",
                "factory",
                "shipping",
                "transport",
                "robot",
                "産業",
                "物流",
                "製造",
                "工場",
            ],
            &["consumer"],
        );
        insert(
            "politics_government",
            &[
                "politics",
                "government",
                "election",
                "policy",
                "law",
                "minister",
                "president",
                "congress",
                "parliament",
                "政治",
                "政府",
                "選挙",
            ],
            &["deal", "apple watch"],
        );
        insert(
            "diplomacy_security",
            &[
                "diplomacy",
                "military",
                "war",
                "defense",
                "treaty",
                "foreign affairs",
                "conflict",
                "nato",
                "外交",
                "軍事",
                "防衛",
                "安全保障",
            ],
            &[],
        );
        insert(
            "law_crime",
            &[
                "law", "crime", "court", "police", "justice", "legal", "judge", "lawsuit",
                "suspect", "prison", "法律", "犯罪", "警察", "裁判",
            ],
            &[],
        );
        insert(
            "education",
            &[
                "education",
                "school",
                "university",
                "student",
                "teacher",
                "learning",
                "campus",
                "curriculum",
                "教育",
                "学校",
                "大学",
                "学生",
            ],
            &[],
        );
        insert(
            "labor_workplace",
            &[
                "labor",
                "workplace",
                "job",
                "career",
                "employment",
                "hr",
                "salary",
                "strike",
                "union",
                "working",
                "労働",
                "職場",
                "雇用",
                "キャリア",
            ],
            &[],
        );
        insert(
            "society_demographics",
            &[
                "society",
                "demographics",
                "population",
                "community",
                "welfare",
                "social",
                "inequality",
                "gender",
                "社会",
                "人口",
                "福祉",
            ],
            &[],
        );
        insert(
            "culture_arts",
            &[
                "culture",
                "art",
                "museum",
                "exhibition",
                "tradition",
                "heritage",
                "painting",
                "sculpture",
                "文化",
                "芸術",
                "美術",
            ],
            &["movie", "music"],
        );
        insert(
            "film_tv",
            &[
                "film",
                "movie",
                "tv",
                "cinema",
                "actor",
                "hollywood",
                "netflix",
                "drama",
                "award",
                "series",
                "映画",
                "テレビ",
                "ドラマ",
            ],
            &[],
        );
        insert(
            "music_audio",
            &[
                "music",
                "song",
                "audio",
                "singer",
                "concert",
                "album",
                "spotify",
                "band",
                "sound",
                "音楽",
                "楽曲",
                "ライブ",
            ],
            &[],
        );
        insert(
            "sports",
            &[
                "sports",
                "football",
                "soccer",
                "baseball",
                "olympics",
                "athlete",
                "team",
                "match",
                "league",
                "game",
                "スポーツ",
                "野球",
                "サッカー",
            ],
            &["esports"],
        );
        insert(
            "food_cuisine",
            &[
                "food",
                "cuisine",
                "restaurant",
                "recipe",
                "chef",
                "cooking",
                "diet",
                "dining",
                "meal",
                "食",
                "料理",
                "グルメ",
            ],
            &[],
        );
        insert(
            "travel_places",
            &[
                "travel",
                "tourism",
                "hotel",
                "destination",
                "flight",
                "airline",
                "vacation",
                "resort",
                "trip",
                "旅行",
                "観光",
                "ホテル",
            ],
            &[],
        );
        insert(
            "home_living",
            &[
                "home",
                "living",
                "interior",
                "house",
                "furniture",
                "lifestyle",
                "garden",
                "diy",
                "decoration",
                "住まい",
                "インテリア",
                "生活",
            ],
            &[],
        );
        insert(
            "games_esports",
            &[
                "game",
                "esports",
                "ps5",
                "nintendo",
                "xbox",
                "gamer",
                "tournament",
                "console",
                "gaming",
                "ゲーム",
                "eスポーツ",
            ],
            &[],
        );
        insert(
            "mobility_automotive",
            &[
                "automotive",
                "car",
                "ev",
                "vehicle",
                "mobility",
                "transport",
                "driving",
                "tesla",
                "toyota",
                "auto",
                "自動車",
                "モビリティ",
                "車",
            ],
            &[],
        );
        insert(
            "consumer_products",
            &[
                "consumer",
                "product",
                "retail",
                "shopping",
                "brand",
                "goods",
                "store",
                "sales",
                "commerce",
                "消費",
                "製品",
                "小売",
                "買い物",
            ],
            &[],
        );

        let mut keywords = HashMap::new();
        let mut negative_keywords = HashMap::new();

        for (id, (pos, neg)) in m {
            keywords.insert(id.clone(), pos);
            negative_keywords.insert(id, neg);
        }

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

        assert!(scores.contains_key("ai_data"));
        assert!(scores.contains_key("health_medicine"));
        assert!(scores["ai_data"] > 0);
    }

    #[test]
    fn score_text_handles_japanese() {
        let keywords = GenreKeywords::default_keywords();
        let text = "この記事では人工知能と機械学習について説明します。";
        let scores = keywords.score_text(text);

        assert!(scores.contains_key("ai_data"));
        assert!(scores["ai_data"] > 0);
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
        // "policy" might trigger politics_government, but "Apple Watch" and "deal" should trigger negative keywords for politics.
        let text = "New Apple Watch deal! Check out the privacy policy update.";
        let scores = keywords.score_text(text);

        // Politics should be excluded despite "policy" being present
        assert!(!scores.contains_key("politics_government"));
    }

    #[test]
    fn negative_keywords_business_exclusion() {
        let keywords = GenreKeywords::default_keywords();
        // "investment" might trigger business (markets_finance), but "art exhibition" should exclude it.
        let text = "The new art exhibition is a great investment of your time.";
        let scores = keywords.score_text(text);

        assert!(!scores.contains_key("markets_finance"));
    }
}
