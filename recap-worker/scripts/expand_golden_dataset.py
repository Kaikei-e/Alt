#!/usr/bin/env python3
"""
Golden Classification Dataset Expansion Script

This script expands the golden dataset according to the plan:
- Updates schema from 2.1 to 2.2
- Adds boundary cases (180 items)
- Adds hard/multi-label cases (50 items)
- Generates parallel JA-EN pairs
- Adds additional baseline items with various styles
- Ensures each genre has minimum 100 items

Usage:
    python expand_golden_dataset.py [--input INPUT_PATH] [--output OUTPUT_PATH]
"""

import argparse
import copy
import json
import random
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional


# Genre definitions with Japanese and English names
GENRES = {
    "ai_data": {"ja": "AI・データ", "en": "AI & Data"},
    "software_dev": {"ja": "ソフトウェア開発", "en": "Software Development"},
    "cybersecurity": {"ja": "サイバーセキュリティ", "en": "Cybersecurity"},
    "consumer_tech": {"ja": "コンシューマーテック", "en": "Consumer Tech"},
    "internet_platforms": {"ja": "インターネット・プラットフォーム", "en": "Internet Platforms"},
    "space_astronomy": {"ja": "宇宙・天文学", "en": "Space & Astronomy"},
    "climate_environment": {"ja": "気候・環境", "en": "Climate & Environment"},
    "energy_transition": {"ja": "エネルギー転換", "en": "Energy Transition"},
    "health_medicine": {"ja": "医療・ヘルスケア", "en": "Healthcare & Medicine"},
    "life_science": {"ja": "生命科学", "en": "Life Science"},
    "economics_macro": {"ja": "マクロ経済", "en": "Macroeconomics"},
    "markets_finance": {"ja": "市場・金融", "en": "Markets & Finance"},
    "startups_innovation": {"ja": "スタートアップ・イノベーション", "en": "Startups & Innovation"},
    "industry_logistics": {"ja": "産業・物流", "en": "Industry & Logistics"},
    "politics_government": {"ja": "政治・行政", "en": "Politics & Government"},
    "diplomacy_security": {"ja": "外交・安全保障", "en": "Diplomacy & Security"},
    "law_crime": {"ja": "法・犯罪", "en": "Law & Crime"},
    "education": {"ja": "教育", "en": "Education"},
    "labor_workplace": {"ja": "労働・職場", "en": "Labor & Workplace"},
    "society_demographics": {"ja": "社会・人口", "en": "Society & Demographics"},
    "culture_arts": {"ja": "文化・芸術", "en": "Culture & Arts"},
    "film_tv": {"ja": "映画・テレビ", "en": "Film & TV"},
    "music_audio": {"ja": "音楽・オーディオ", "en": "Music & Audio"},
    "sports": {"ja": "スポーツ", "en": "Sports"},
    "food_cuisine": {"ja": "食・グルメ", "en": "Food & Cuisine"},
    "travel_places": {"ja": "旅行・地域", "en": "Travel & Places"},
    "home_living": {"ja": "住まい・生活", "en": "Home & Living"},
    "games_esports": {"ja": "ゲーム・eスポーツ", "en": "Games & Esports"},
    "mobility_automotive": {"ja": "モビリティ・自動車", "en": "Mobility & Automotive"},
    "consumer_products": {"ja": "消費・製品", "en": "Consumer & Products"},
}

# Boundary case pairs (genre_a, genre_b, count, example_topic_ja, example_topic_en)
BOUNDARY_PAIRS = [
    ("ai_data", "software_dev", 20, "MLOpsパイプライン構築", "MLOps pipeline development"),
    ("health_medicine", "life_science", 20, "臨床試験の基礎研究", "Clinical trial basic research"),
    ("markets_finance", "economics_macro", 20, "中央銀行の金利政策", "Central bank interest rate policy"),
    ("politics_government", "diplomacy_security", 20, "外交政策の国内影響", "Domestic impact of foreign policy"),
    ("consumer_tech", "internet_platforms", 20, "SNSアプリの新機能", "Social media app new features"),
    ("climate_environment", "energy_transition", 20, "再エネと環境保全", "Renewables and environmental conservation"),
    ("film_tv", "music_audio", 15, "映画サントラ制作", "Film soundtrack production"),
    ("games_esports", "consumer_tech", 15, "ゲーミングデバイス", "Gaming devices"),
    ("startups_innovation", "markets_finance", 15, "スタートアップIPO", "Startup IPO"),
    ("industry_logistics", "mobility_automotive", 15, "自動車サプライチェーン", "Automotive supply chain"),
]

# Hard/multi-label case combinations
HARD_COMBINATIONS = [
    (["ai_data", "health_medicine"], 10, "AI診断システム臨床導入", "AI diagnostic system clinical deployment"),
    (["cybersecurity", "law_crime"], 10, "ランサムウェア訴訟", "Ransomware litigation"),
    (["climate_environment", "politics_government"], 10, "気候政策サミット", "Climate policy summit"),
    (["education", "labor_workplace"], 10, "リスキリング職業訓練", "Reskilling job training"),
    (["space_astronomy", "diplomacy_security"], 10, "宇宙軍・衛星監視", "Space force satellite surveillance"),
]

# Templates for generating boundary cases
BOUNDARY_TEMPLATES_JA = [
    "{topic}に関する新たな取り組みが発表された。{genre_a_aspect}と{genre_b_aspect}の両面から注目されている。",
    "{topic}の最新動向が明らかになった。専門家は{genre_a_aspect}としての側面と{genre_b_aspect}としての意義を指摘する。",
    "{topic}をめぐり、{genre_a_aspect}の観点と{genre_b_aspect}の視点からの議論が活発化している。",
    "新たな{topic}が登場し、{genre_a_aspect}と{genre_b_aspect}の境界を曖昧にしている。",
    "{topic}の進展により、{genre_a_aspect}と{genre_b_aspect}の融合が進んでいる。",
]

BOUNDARY_TEMPLATES_EN = [
    "A new initiative on {topic} was announced, drawing attention from both {genre_a_aspect} and {genre_b_aspect} perspectives.",
    "Latest developments in {topic} have emerged. Experts point to its significance as both {genre_a_aspect} and {genre_b_aspect}.",
    "Active discussions on {topic} are underway from both {genre_a_aspect} and {genre_b_aspect} viewpoints.",
    "New {topic} is blurring the boundaries between {genre_a_aspect} and {genre_b_aspect}.",
    "Advances in {topic} are driving convergence between {genre_a_aspect} and {genre_b_aspect}.",
]

# Templates for hard/multi-label cases
HARD_TEMPLATES_JA = [
    "{topic}が{genres_list}の分野で注目を集めている。複合的な影響が予想される。",
    "{topic}は{genres_list}にまたがる課題として認識されている。",
    "{genres_list}の交差点に位置する{topic}が議論の的となっている。",
]

HARD_TEMPLATES_EN = [
    "{topic} is attracting attention across {genres_list} fields. Complex impacts are expected.",
    "{topic} is recognized as an issue spanning {genres_list}.",
    "{topic}, situated at the intersection of {genres_list}, has become a focus of discussion.",
]

# Style-specific templates
HEADLINE_TEMPLATES_JA = [
    "{action}、{result}",
    "【速報】{topic}、{outcome}",
    "{subject}が{action}へ",
]

HEADLINE_TEMPLATES_EN = [
    "{subject} {action}: {result}",
    "Breaking: {topic} {outcome}",
    "{subject} moves to {action}",
]

LEAD_TEMPLATES_JA = [
    "{when}、{who}は{what}を発表した。{why}が背景にある。{how}で実施される予定だ。",
    "{who}が{when}、{what}を開始した。{where}で行われ、{how_many}が参加する見込み。",
]

LEAD_TEMPLATES_EN = [
    "On {when}, {who} announced {what}. This comes amid {why}. It will be implemented {how}.",
    "{who} launched {what} on {when}. Taking place in {where}, it expects {how_many} participants.",
]

LONG_FORM_TEMPLATES_JA = [
    "{intro}。{background}。{development}。専門家は「{quote}」と述べている。今後の展開が注目される。",
]

LONG_FORM_TEMPLATES_EN = [
    "{intro}. {background}. {development}. Experts say, \"{quote}.\" Future developments are being closely watched.",
]


@dataclass
class DatasetItem:
    """Represents a single item in the golden dataset."""
    id: str
    content_ja: Optional[str] = None
    content_en: Optional[str] = None
    expected_genres: list = field(default_factory=list)
    primary_genre: str = ""
    difficulty: str = "baseline"
    language_pairing: str = "none"
    source: str = "synthetic_v6"
    notes: str = ""
    secondary_genres: list = field(default_factory=list)
    boundary_pair: list = field(default_factory=list)
    style: Optional[str] = None
    terminology_density: Optional[str] = None
    parallel_id: Optional[str] = None

    def to_dict(self) -> dict:
        """Convert to dictionary for JSON serialization."""
        d = {
            "id": self.id,
            "content_ja": self.content_ja,
            "content_en": self.content_en,
            "expected_genres": self.expected_genres,
            "primary_genre": self.primary_genre,
            "difficulty": self.difficulty,
            "language_pairing": self.language_pairing,
            "source": self.source,
            "notes": self.notes,
        }
        # Add optional fields only if they have values
        if self.secondary_genres:
            d["secondary_genres"] = self.secondary_genres
        if self.boundary_pair:
            d["boundary_pair"] = self.boundary_pair
        if self.style:
            d["style"] = self.style
        if self.terminology_density:
            d["terminology_density"] = self.terminology_density
        if self.parallel_id:
            d["parallel_id"] = self.parallel_id
        return d


def generate_boundary_item(
    genre_a: str,
    genre_b: str,
    topic_ja: str,
    topic_en: str,
    idx: int,
    lang: str = "ja"
) -> DatasetItem:
    """Generate a boundary case item."""
    genre_a_name = GENRES[genre_a]
    genre_b_name = GENRES[genre_b]

    if lang == "ja":
        template = random.choice(BOUNDARY_TEMPLATES_JA)
        content = template.format(
            topic=topic_ja,
            genre_a_aspect=genre_a_name["ja"],
            genre_b_aspect=genre_b_name["ja"]
        )
        return DatasetItem(
            id=f"{genre_a}-boundary-{genre_b}-{idx:02d}-ja",
            content_ja=content,
            content_en=None,
            expected_genres=[genre_a],
            primary_genre=genre_a,
            difficulty="boundary",
            language_pairing="ja_only",
            source="synthetic_v6",
            notes=f"Boundary case: {genre_a} vs {genre_b}",
            boundary_pair=[genre_a, genre_b],
        )
    else:
        template = random.choice(BOUNDARY_TEMPLATES_EN)
        content = template.format(
            topic=topic_en,
            genre_a_aspect=genre_a_name["en"],
            genre_b_aspect=genre_b_name["en"]
        )
        return DatasetItem(
            id=f"{genre_a}-boundary-{genre_b}-{idx:02d}-en",
            content_ja=None,
            content_en=content,
            expected_genres=[genre_a],
            primary_genre=genre_a,
            difficulty="boundary",
            language_pairing="en_only",
            source="synthetic_v6",
            notes=f"Boundary case: {genre_a} vs {genre_b}",
            boundary_pair=[genre_a, genre_b],
        )


def generate_hard_item(
    genres: list,
    topic_ja: str,
    topic_en: str,
    idx: int,
    lang: str = "ja"
) -> DatasetItem:
    """Generate a hard/multi-label case item."""
    primary = genres[0]
    secondary = genres[1:]

    if lang == "ja":
        genres_list = "、".join([GENRES[g]["ja"] for g in genres])
        template = random.choice(HARD_TEMPLATES_JA)
        content = template.format(topic=topic_ja, genres_list=genres_list)
        return DatasetItem(
            id=f"{primary}-hard-{idx:02d}-ja",
            content_ja=content,
            content_en=None,
            expected_genres=genres,
            primary_genre=primary,
            difficulty="hard",
            language_pairing="ja_only",
            source="synthetic_v6",
            notes=f"Multi-label hard case: {', '.join(genres)}",
            secondary_genres=secondary,
        )
    else:
        genres_list = ", ".join([GENRES[g]["en"] for g in genres])
        template = random.choice(HARD_TEMPLATES_EN)
        content = template.format(topic=topic_en, genres_list=genres_list)
        return DatasetItem(
            id=f"{primary}-hard-{idx:02d}-en",
            content_ja=None,
            content_en=content,
            expected_genres=genres,
            primary_genre=primary,
            difficulty="hard",
            language_pairing="en_only",
            source="synthetic_v6",
            notes=f"Multi-label hard case: {', '.join(genres)}",
            secondary_genres=secondary,
        )


# Topic variations for each genre (for baseline expansion)
GENRE_TOPICS = {
    "ai_data": {
        "ja": [
            "大規模言語モデルの学習効率化",
            "マルチモーダルAIの実用化",
            "AIの透明性確保ガイドライン",
            "データプライバシー保護技術",
            "分散学習プラットフォーム",
            "生成AIの著作権問題",
            "AI品質評価フレームワーク",
            "エッジAIの推論高速化",
        ],
        "en": [
            "Large language model training efficiency",
            "Multimodal AI commercialization",
            "AI transparency guidelines",
            "Data privacy protection technology",
            "Distributed learning platform",
            "Generative AI copyright issues",
            "AI quality evaluation framework",
            "Edge AI inference acceleration",
        ],
    },
    "software_dev": {
        "ja": [
            "コンテナオーケストレーション",
            "サーバーレスアーキテクチャ",
            "マイクロサービス設計",
            "CI/CDパイプライン最適化",
            "コードレビュー自動化",
            "テスト駆動開発の普及",
            "オブザーバビリティ基盤",
            "APIバージョニング戦略",
        ],
        "en": [
            "Container orchestration",
            "Serverless architecture",
            "Microservices design",
            "CI/CD pipeline optimization",
            "Automated code review",
            "Test-driven development adoption",
            "Observability infrastructure",
            "API versioning strategy",
        ],
    },
    "cybersecurity": {
        "ja": [
            "ゼロトラストセキュリティ",
            "脆弱性スキャン自動化",
            "インシデント対応訓練",
            "フィッシング対策強化",
            "暗号化技術の進化",
            "セキュリティ監査基準",
            "ランサムウェア対策",
            "認証基盤の近代化",
        ],
        "en": [
            "Zero trust security",
            "Automated vulnerability scanning",
            "Incident response training",
            "Phishing countermeasures",
            "Encryption technology evolution",
            "Security audit standards",
            "Ransomware protection",
            "Authentication infrastructure modernization",
        ],
    },
    "consumer_tech": {
        "ja": [
            "折りたたみスマートフォン",
            "ワイヤレスイヤホン進化",
            "スマートホームデバイス",
            "AR/VRヘッドセット",
            "電子書籍リーダー",
            "スマートウォッチ健康機能",
            "ポータブルプロジェクター",
            "音声アシスタント",
        ],
        "en": [
            "Foldable smartphones",
            "Wireless earbuds evolution",
            "Smart home devices",
            "AR/VR headsets",
            "E-reader devices",
            "Smartwatch health features",
            "Portable projectors",
            "Voice assistants",
        ],
    },
    "internet_platforms": {
        "ja": [
            "SNSアルゴリズム変更",
            "動画配信サービス競争",
            "クラウドゲーミング",
            "音声SNSの台頭",
            "アプリストア規制",
            "クリエイターエコノミー",
            "ライブコマース",
            "メタバースプラットフォーム",
        ],
        "en": [
            "Social media algorithm changes",
            "Streaming service competition",
            "Cloud gaming",
            "Audio social networks rise",
            "App store regulations",
            "Creator economy",
            "Live commerce",
            "Metaverse platforms",
        ],
    },
    "space_astronomy": {
        "ja": [
            "火星探査ミッション",
            "小惑星サンプルリターン",
            "宇宙ステーション商業化",
            "衛星コンステレーション",
            "月面基地計画",
            "系外惑星発見",
            "宇宙デブリ対策",
            "深宇宙通信技術",
        ],
        "en": [
            "Mars exploration mission",
            "Asteroid sample return",
            "Space station commercialization",
            "Satellite constellation",
            "Lunar base plans",
            "Exoplanet discovery",
            "Space debris mitigation",
            "Deep space communication",
        ],
    },
    "climate_environment": {
        "ja": [
            "森林再生プロジェクト",
            "海洋プラスチック対策",
            "生物多様性保全",
            "気候変動適応策",
            "大気汚染モニタリング",
            "湿地保護活動",
            "サンゴ礁回復",
            "持続可能な農業",
        ],
        "en": [
            "Forest restoration project",
            "Ocean plastic countermeasures",
            "Biodiversity conservation",
            "Climate change adaptation",
            "Air pollution monitoring",
            "Wetland protection",
            "Coral reef recovery",
            "Sustainable agriculture",
        ],
    },
    "energy_transition": {
        "ja": [
            "洋上風力発電",
            "次世代蓄電池",
            "水素エネルギー",
            "スマートグリッド",
            "太陽光発電効率化",
            "地熱発電開発",
            "EV充電インフラ",
            "脱炭素化ロードマップ",
        ],
        "en": [
            "Offshore wind power",
            "Next-gen batteries",
            "Hydrogen energy",
            "Smart grid",
            "Solar efficiency improvements",
            "Geothermal development",
            "EV charging infrastructure",
            "Decarbonization roadmap",
        ],
    },
    "health_medicine": {
        "ja": [
            "遠隔医療の普及",
            "予防医療プログラム",
            "医療データ連携",
            "新薬承認プロセス",
            "病院経営効率化",
            "感染症対策体制",
            "高齢者医療の課題",
            "医療人材育成",
        ],
        "en": [
            "Telemedicine adoption",
            "Preventive care programs",
            "Healthcare data integration",
            "Drug approval process",
            "Hospital management efficiency",
            "Infectious disease preparedness",
            "Elderly care challenges",
            "Medical workforce development",
        ],
    },
    "life_science": {
        "ja": [
            "遺伝子編集技術",
            "再生医療研究",
            "がん免疫療法",
            "タンパク質構造解析",
            "バイオマーカー発見",
            "幹細胞研究",
            "マイクロバイオーム解析",
            "合成生物学",
        ],
        "en": [
            "Gene editing technology",
            "Regenerative medicine research",
            "Cancer immunotherapy",
            "Protein structure analysis",
            "Biomarker discovery",
            "Stem cell research",
            "Microbiome analysis",
            "Synthetic biology",
        ],
    },
    "economics_macro": {
        "ja": [
            "インフレ対策",
            "失業率の推移",
            "為替市場動向",
            "GDP成長率予測",
            "財政政策の効果",
            "国際収支の変化",
            "消費者信頼感指数",
            "金融緩和政策",
        ],
        "en": [
            "Inflation countermeasures",
            "Unemployment rate trends",
            "Currency market movements",
            "GDP growth forecasts",
            "Fiscal policy effects",
            "Balance of payments changes",
            "Consumer confidence index",
            "Monetary easing policy",
        ],
    },
    "markets_finance": {
        "ja": [
            "株式市場の変動",
            "企業M&A動向",
            "債券利回り推移",
            "IPO市場分析",
            "ヘッジファンド戦略",
            "ESG投資拡大",
            "暗号資産規制",
            "決算発表シーズン",
        ],
        "en": [
            "Stock market volatility",
            "Corporate M&A trends",
            "Bond yield movements",
            "IPO market analysis",
            "Hedge fund strategies",
            "ESG investment expansion",
            "Cryptocurrency regulations",
            "Earnings season",
        ],
    },
    "startups_innovation": {
        "ja": [
            "シード投資動向",
            "スタートアップエコシステム",
            "アクセラレータープログラム",
            "ユニコーン企業分析",
            "技術系創業支援",
            "オープンイノベーション",
            "社内ベンチャー制度",
            "大学発スタートアップ",
        ],
        "en": [
            "Seed investment trends",
            "Startup ecosystem",
            "Accelerator programs",
            "Unicorn company analysis",
            "Tech startup support",
            "Open innovation",
            "Corporate venture programs",
            "University spinoffs",
        ],
    },
    "industry_logistics": {
        "ja": [
            "製造業のDX",
            "サプライチェーン強靱化",
            "物流自動化",
            "工場のスマート化",
            "在庫管理最適化",
            "倉庫ロボット導入",
            "ラストマイル配送",
            "製造コスト削減",
        ],
        "en": [
            "Manufacturing DX",
            "Supply chain resilience",
            "Logistics automation",
            "Smart factory",
            "Inventory optimization",
            "Warehouse robotics",
            "Last-mile delivery",
            "Manufacturing cost reduction",
        ],
    },
    "politics_government": {
        "ja": [
            "選挙制度改革",
            "地方分権推進",
            "行政デジタル化",
            "国会審議動向",
            "政党支持率変化",
            "政策立案過程",
            "規制改革議論",
            "首相記者会見",
        ],
        "en": [
            "Electoral reform",
            "Decentralization promotion",
            "Digital government",
            "Parliamentary proceedings",
            "Party approval ratings",
            "Policy formulation process",
            "Regulatory reform debate",
            "Prime minister press conference",
        ],
    },
    "diplomacy_security": {
        "ja": [
            "二国間首脳会談",
            "安全保障条約",
            "軍事演習実施",
            "国際紛争調停",
            "防衛費予算",
            "同盟関係強化",
            "核不拡散条約",
            "地域安全保障",
        ],
        "en": [
            "Bilateral summit meeting",
            "Security treaty",
            "Military exercises",
            "International conflict mediation",
            "Defense budget",
            "Alliance strengthening",
            "Non-proliferation treaty",
            "Regional security",
        ],
    },
    "law_crime": {
        "ja": [
            "法改正審議",
            "裁判判例分析",
            "サイバー犯罪対策",
            "消費者保護法",
            "知的財産権訴訟",
            "刑事司法改革",
            "企業コンプライアンス",
            "個人情報保護法",
        ],
        "en": [
            "Legal reform deliberations",
            "Court precedent analysis",
            "Cybercrime countermeasures",
            "Consumer protection law",
            "IP litigation",
            "Criminal justice reform",
            "Corporate compliance",
            "Data protection law",
        ],
    },
    "education": {
        "ja": [
            "オンライン授業の進化",
            "STEM教育推進",
            "大学入試改革",
            "教員働き方改革",
            "EdTech活用",
            "グローバル教育",
            "特別支援教育",
            "社会人教育",
        ],
        "en": [
            "Online learning evolution",
            "STEM education promotion",
            "University entrance reform",
            "Teacher work-style reform",
            "EdTech adoption",
            "Global education",
            "Special needs education",
            "Adult education",
        ],
    },
    "labor_workplace": {
        "ja": [
            "リモートワーク定着",
            "最低賃金引き上げ",
            "働き方多様化",
            "副業解禁動向",
            "ハラスメント対策",
            "人材獲得競争",
            "労働組合活動",
            "ジョブ型雇用",
        ],
        "en": [
            "Remote work adoption",
            "Minimum wage increase",
            "Work style diversification",
            "Side job policy changes",
            "Harassment prevention",
            "Talent acquisition competition",
            "Labor union activities",
            "Job-based employment",
        ],
    },
    "society_demographics": {
        "ja": [
            "少子高齢化対策",
            "移民政策議論",
            "都市部人口集中",
            "地域活性化事業",
            "社会福祉制度",
            "格差問題",
            "コミュニティ支援",
            "高齢者見守り",
        ],
        "en": [
            "Aging society measures",
            "Immigration policy debate",
            "Urban population concentration",
            "Regional revitalization",
            "Social welfare system",
            "Inequality issues",
            "Community support",
            "Elderly monitoring",
        ],
    },
    "culture_arts": {
        "ja": [
            "美術展開催",
            "伝統工芸振興",
            "現代アート市場",
            "舞台芸術公演",
            "文化財保護",
            "アーティスト支援",
            "芸術教育プログラム",
            "国際文化交流",
        ],
        "en": [
            "Art exhibition opening",
            "Traditional crafts promotion",
            "Contemporary art market",
            "Performing arts",
            "Cultural heritage protection",
            "Artist support",
            "Arts education program",
            "International cultural exchange",
        ],
    },
    "film_tv": {
        "ja": [
            "映画興行収入",
            "ドラマ視聴率",
            "配信オリジナル作品",
            "映画祭受賞",
            "アニメ制作動向",
            "ドキュメンタリー",
            "テレビ局経営",
            "映像技術革新",
        ],
        "en": [
            "Box office revenue",
            "TV drama ratings",
            "Streaming original content",
            "Film festival awards",
            "Anime production trends",
            "Documentary",
            "TV network management",
            "Video technology innovation",
        ],
    },
    "music_audio": {
        "ja": [
            "音楽配信サービス",
            "コンサートツアー",
            "新人アーティスト",
            "音楽フェス開催",
            "レコード会社動向",
            "ポッドキャスト人気",
            "オーディオ機器",
            "音楽著作権管理",
        ],
        "en": [
            "Music streaming service",
            "Concert tour",
            "New artist debut",
            "Music festival",
            "Record label trends",
            "Podcast popularity",
            "Audio equipment",
            "Music rights management",
        ],
    },
    "sports": {
        "ja": [
            "プロ野球シーズン",
            "サッカー国際大会",
            "オリンピック準備",
            "選手移籍情報",
            "スポーツビジネス",
            "アマチュア競技",
            "競技ルール変更",
            "スポーツ科学",
        ],
        "en": [
            "Baseball season",
            "Soccer international tournament",
            "Olympic preparations",
            "Player transfer news",
            "Sports business",
            "Amateur sports",
            "Rule changes",
            "Sports science",
        ],
    },
    "food_cuisine": {
        "ja": [
            "レストラン新規オープン",
            "食品トレンド分析",
            "料理レシピ紹介",
            "飲食店経営",
            "フードテック",
            "地産地消推進",
            "食品安全基準",
            "グルメイベント",
        ],
        "en": [
            "Restaurant opening",
            "Food trend analysis",
            "Recipe introduction",
            "Restaurant management",
            "Food tech",
            "Local production promotion",
            "Food safety standards",
            "Gourmet event",
        ],
    },
    "travel_places": {
        "ja": [
            "観光地再開発",
            "旅行需要回復",
            "インバウンド誘致",
            "地域の魅力発信",
            "宿泊施設動向",
            "観光DX推進",
            "エコツーリズム",
            "交通アクセス改善",
        ],
        "en": [
            "Tourist area redevelopment",
            "Travel demand recovery",
            "Inbound tourism promotion",
            "Regional attraction marketing",
            "Accommodation trends",
            "Tourism DX",
            "Ecotourism",
            "Transportation access improvement",
        ],
    },
    "home_living": {
        "ja": [
            "住宅市場動向",
            "インテリアトレンド",
            "スマートホーム導入",
            "DIY人気",
            "家事効率化",
            "収納術",
            "リフォーム市場",
            "賃貸住宅事情",
        ],
        "en": [
            "Housing market trends",
            "Interior design trends",
            "Smart home adoption",
            "DIY popularity",
            "Household efficiency",
            "Storage solutions",
            "Renovation market",
            "Rental housing conditions",
        ],
    },
    "games_esports": {
        "ja": [
            "新作ゲーム発売",
            "eスポーツ大会",
            "ゲーム実況人気",
            "ゲーム開発会社",
            "モバイルゲーム市場",
            "ゲーム規制議論",
            "VRゲーム進化",
            "ゲームコミュニティ",
        ],
        "en": [
            "New game release",
            "Esports tournament",
            "Game streaming popularity",
            "Game development company",
            "Mobile game market",
            "Gaming regulation debate",
            "VR gaming evolution",
            "Gaming community",
        ],
    },
    "mobility_automotive": {
        "ja": [
            "電気自動車販売",
            "自動運転技術",
            "カーシェアリング",
            "次世代モビリティ",
            "自動車メーカー戦略",
            "交通渋滞対策",
            "充電インフラ整備",
            "車両安全技術",
        ],
        "en": [
            "Electric vehicle sales",
            "Autonomous driving technology",
            "Car sharing",
            "Next-gen mobility",
            "Automaker strategy",
            "Traffic congestion measures",
            "Charging infrastructure",
            "Vehicle safety technology",
        ],
    },
    "consumer_products": {
        "ja": [
            "日用品価格動向",
            "新商品発売",
            "ブランド戦略",
            "消費者行動分析",
            "サブスクサービス",
            "ECサイト競争",
            "パッケージデザイン",
            "消費トレンド",
        ],
        "en": [
            "Daily goods price trends",
            "New product launch",
            "Brand strategy",
            "Consumer behavior analysis",
            "Subscription services",
            "E-commerce competition",
            "Package design",
            "Consumption trends",
        ],
    },
}

# Baseline content templates
BASELINE_TEMPLATES_JA = [
    "{topic}が注目を集めている。専門家は今後の動向を注視している。",
    "{topic}に関する新たな取り組みが発表された。業界関係者は期待を寄せている。",
    "{topic}の最新動向が明らかになった。影響の広がりが見込まれる。",
    "{topic}をめぐり、議論が活発化している。各方面からの意見が寄せられている。",
    "{topic}について、新たな研究結果が発表された。今後の展開が注目される。",
]

BASELINE_TEMPLATES_EN = [
    "{topic} is attracting attention. Experts are closely watching future developments.",
    "A new initiative on {topic} was announced. Industry stakeholders are hopeful.",
    "Latest developments on {topic} have emerged. Widespread impact is expected.",
    "Discussions on {topic} are heating up. Opinions from various quarters are being shared.",
    "New research findings on {topic} were released. Future developments are being watched.",
]


def generate_baseline_item(genre: str, topic_ja: str, topic_en: str, idx: int, lang: str, style: str = None) -> DatasetItem:
    """Generate a baseline item."""
    if lang == "ja":
        template = random.choice(BASELINE_TEMPLATES_JA)
        content = template.format(topic=topic_ja)
        return DatasetItem(
            id=f"{genre}-baseline-{idx:03d}-ja",
            content_ja=content,
            content_en=None,
            expected_genres=[genre],
            primary_genre=genre,
            difficulty="baseline",
            language_pairing="ja_only",
            source="synthetic_v6",
            notes="Baseline expansion",
            style=style,
        )
    else:
        template = random.choice(BASELINE_TEMPLATES_EN)
        content = template.format(topic=topic_en)
        return DatasetItem(
            id=f"{genre}-baseline-{idx:03d}-en",
            content_ja=None,
            content_en=content,
            expected_genres=[genre],
            primary_genre=genre,
            difficulty="baseline",
            language_pairing="en_only",
            source="synthetic_v6",
            notes="Baseline expansion",
            style=style,
        )


def generate_parallel_item(ja_item: dict, idx: int) -> tuple[DatasetItem, DatasetItem]:
    """Generate a parallel JA-EN pair from existing JA item."""
    # Create translations based on templates
    genre = ja_item.get("primary_genre", ja_item.get("expected_genres", ["unknown"])[0])
    ja_content = ja_item.get("content_ja", "")

    if not ja_content:
        return None, None

    # Get genre topics for translation context
    topics = GENRE_TOPICS.get(genre, {"en": ["topic"]})
    en_topic = random.choice(topics["en"])

    # Generate English content using template
    template = random.choice(BASELINE_TEMPLATES_EN)
    en_content = template.format(topic=en_topic)

    parallel_id = f"parallel-{idx:04d}"

    ja_new = DatasetItem(
        id=f"{genre}-parallel-{idx:04d}-ja",
        content_ja=ja_content,
        content_en=None,
        expected_genres=ja_item.get("expected_genres", [genre]),
        primary_genre=genre,
        difficulty=ja_item.get("difficulty", "baseline"),
        language_pairing="parallel",
        source="synthetic_v6",
        notes="Parallel pair (JA)",
        parallel_id=parallel_id,
    )

    en_new = DatasetItem(
        id=f"{genre}-parallel-{idx:04d}-en",
        content_ja=None,
        content_en=en_content,
        expected_genres=ja_item.get("expected_genres", [genre]),
        primary_genre=genre,
        difficulty=ja_item.get("difficulty", "baseline"),
        language_pairing="parallel",
        source="synthetic_v6",
        notes="Parallel pair (EN)",
        parallel_id=parallel_id,
    )

    return ja_new, en_new


def expand_dataset(input_path: Path, output_path: Path):
    """Main function to expand the golden dataset."""
    print(f"Loading dataset from {input_path}...")
    with open(input_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    existing_items = data["items"]
    print(f"Loaded {len(existing_items)} existing items")

    # Count items per genre
    genre_counts = {}
    for item in existing_items:
        genre = item.get("primary_genre", item.get("expected_genres", ["unknown"])[0])
        genre_counts[genre] = genre_counts.get(genre, 0) + 1

    print("\nCurrent genre distribution:")
    for genre, count in sorted(genre_counts.items()):
        print(f"  {genre}: {count}")

    new_items = []

    # 1. Generate boundary cases
    print("\nGenerating boundary cases...")
    boundary_idx = 0
    for genre_a, genre_b, count, topic_ja, topic_en in BOUNDARY_PAIRS:
        for i in range(count):
            # Alternate between JA and EN, and alternate primary genre
            lang = "ja" if i % 2 == 0 else "en"
            primary = genre_a if i % 4 < 2 else genre_b

            # Generate varied topics
            topics_a = GENRE_TOPICS.get(genre_a, {"ja": [topic_ja], "en": [topic_en]})
            topics_b = GENRE_TOPICS.get(genre_b, {"ja": [topic_ja], "en": [topic_en]})

            if lang == "ja":
                varied_topic_ja = random.choice(topics_a["ja"]) + "と" + random.choice(topics_b["ja"])
                item = generate_boundary_item(primary, genre_b if primary == genre_a else genre_a,
                                              varied_topic_ja, topic_en, boundary_idx, lang)
            else:
                varied_topic_en = random.choice(topics_a["en"]) + " and " + random.choice(topics_b["en"])
                item = generate_boundary_item(primary, genre_b if primary == genre_a else genre_a,
                                              topic_ja, varied_topic_en, boundary_idx, lang)

            new_items.append(item.to_dict())
            boundary_idx += 1

    print(f"  Generated {boundary_idx} boundary items")

    # 2. Generate hard/multi-label cases
    print("\nGenerating hard cases...")
    hard_idx = 0
    for genres, count, topic_ja, topic_en in HARD_COMBINATIONS:
        for i in range(count):
            lang = "ja" if i % 2 == 0 else "en"
            item = generate_hard_item(genres, topic_ja, topic_en, hard_idx, lang)
            new_items.append(item.to_dict())
            hard_idx += 1

    print(f"  Generated {hard_idx} hard items")

    # 3. Generate parallel pairs from existing JA items
    print("\nGenerating parallel pairs...")
    ja_items = [item for item in existing_items if item.get("content_ja") and not item.get("content_en")]

    # Select items for parallel pair generation (30% target)
    target_parallel = int(len(existing_items) * 0.30)
    selected_for_parallel = random.sample(ja_items, min(target_parallel // 2, len(ja_items)))

    parallel_idx = 0
    for ja_item in selected_for_parallel:
        ja_new, en_new = generate_parallel_item(ja_item, parallel_idx)
        if ja_new and en_new:
            new_items.append(ja_new.to_dict())
            new_items.append(en_new.to_dict())
            parallel_idx += 1

    print(f"  Generated {parallel_idx * 2} parallel items ({parallel_idx} pairs)")

    # 4. Generate additional baseline items to reach target
    print("\nGenerating additional baseline items...")
    target_per_genre = 120  # Target items per genre
    baseline_idx = 0

    # Recalculate genre counts including new items
    for item in new_items:
        genre = item.get("primary_genre", item.get("expected_genres", ["unknown"])[0])
        genre_counts[genre] = genre_counts.get(genre, 0) + 1

    for genre in GENRES.keys():
        current = genre_counts.get(genre, 0)
        needed = max(0, target_per_genre - current)

        if needed > 0:
            topics = GENRE_TOPICS.get(genre, {"ja": ["トピック"], "en": ["topic"]})

            for i in range(needed):
                lang = "ja" if i % 2 == 0 else "en"
                topic_list = topics["ja" if lang == "ja" else "en"]
                topic = topic_list[i % len(topic_list)]

                styles = [None, "headline", "lead", "long_form"]
                style = styles[i % len(styles)]

                item = generate_baseline_item(
                    genre,
                    topic if lang == "ja" else "topic",
                    topic if lang == "en" else "topic",
                    baseline_idx,
                    lang,
                    style
                )
                new_items.append(item.to_dict())
                baseline_idx += 1

    print(f"  Generated {baseline_idx} additional baseline items")

    # Update existing items with new fields
    print("\nUpdating existing items with new schema fields...")
    updated_existing = []
    for item in existing_items:
        updated_item = copy.deepcopy(item)

        # Ensure difficulty field exists
        if "difficulty" not in updated_item:
            updated_item["difficulty"] = "baseline"

        # Update language_pairing based on content
        has_ja = updated_item.get("content_ja") and updated_item["content_ja"].strip()
        has_en = updated_item.get("content_en") and updated_item["content_en"].strip()

        if has_ja and has_en:
            updated_item["language_pairing"] = "parallel"
        elif has_ja:
            updated_item["language_pairing"] = "ja_only"
        elif has_en:
            updated_item["language_pairing"] = "en_only"
        else:
            updated_item["language_pairing"] = "none"

        updated_existing.append(updated_item)

    # Combine all items
    all_items = updated_existing + new_items

    # Create output data with updated schema
    output_data = {
        "schema_version": "2.2",
        "taxonomy_version": data.get("taxonomy_version", "genre-fixed-30-v1"),
        "description": "Expanded golden set with boundary cases, hard cases, and parallel pairs. Schema 2.2 with difficulty levels and language pairing metadata.",
        "genres": data.get("genres", []),
        "items": all_items,
    }

    # Write output
    print(f"\nWriting expanded dataset to {output_path}...")
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(output_data, f, ensure_ascii=False, indent=2)

    # Print final statistics
    print("\n=== Final Statistics ===")
    print(f"Total items: {len(all_items)}")

    # Difficulty distribution
    difficulties = {}
    for item in all_items:
        diff = item.get("difficulty", "baseline")
        difficulties[diff] = difficulties.get(diff, 0) + 1
    print("\nDifficulty distribution:")
    for diff, count in sorted(difficulties.items()):
        pct = count / len(all_items) * 100
        print(f"  {diff}: {count} ({pct:.1f}%)")

    # Language pairing distribution
    pairings = {}
    for item in all_items:
        pairing = item.get("language_pairing", "none")
        pairings[pairing] = pairings.get(pairing, 0) + 1
    print("\nLanguage pairing distribution:")
    for pairing, count in sorted(pairings.items()):
        pct = count / len(all_items) * 100
        print(f"  {pairing}: {count} ({pct:.1f}%)")

    # Genre distribution
    final_genre_counts = {}
    for item in all_items:
        genre = item.get("primary_genre", item.get("expected_genres", ["unknown"])[0])
        final_genre_counts[genre] = final_genre_counts.get(genre, 0) + 1
    print("\nGenre distribution:")
    for genre, count in sorted(final_genre_counts.items()):
        print(f"  {genre}: {count}")

    min_count = min(final_genre_counts.values())
    max_count = max(final_genre_counts.values())
    avg_count = sum(final_genre_counts.values()) / len(final_genre_counts)
    print(f"\nGenre stats: min={min_count}, max={max_count}, avg={avg_count:.1f}")

    print("\nDone!")


def main():
    parser = argparse.ArgumentParser(description="Expand Golden Classification Dataset")
    parser.add_argument(
        "--input", "-i",
        default="recap-worker/recap-worker/tests/data/golden_classification.json",
        help="Input golden dataset path"
    )
    parser.add_argument(
        "--output", "-o",
        default=None,
        help="Output path (defaults to overwriting input)"
    )

    args = parser.parse_args()

    input_path = Path(args.input)
    if not input_path.is_absolute():
        # Try to find the file relative to script or cwd
        candidates = [
            Path(args.input),
            Path(__file__).parent.parent / args.input,
            Path.cwd() / args.input,
        ]
        for candidate in candidates:
            if candidate.exists():
                input_path = candidate
                break

    if not input_path.exists():
        print(f"Error: Input file not found: {input_path}")
        return 1

    output_path = Path(args.output) if args.output else input_path

    expand_dataset(input_path, output_path)
    return 0


if __name__ == "__main__":
    exit(main())
