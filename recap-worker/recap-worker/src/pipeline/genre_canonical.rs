use std::collections::HashMap;

use once_cell::sync::Lazy;

/// Canonical sentences for each genre to be used for embedding-based filtering.
/// These sentences represent the "ideal" content for a genre.
#[allow(dead_code)] // May be used in future refactoring
static CANONICAL_SENTENCES: Lazy<HashMap<&'static str, Vec<&'static str>>> = Lazy::new(|| {
    let mut m = HashMap::new();

    m.insert(
        "ai_data",
        vec![
            "Machine learning, generative AI, data analytics, AI applications, model development and large-scale data infrastructure.",
            "機械学習、生成AI、データ分析、AI活用事例、モデル開発や大規模データ基盤に関する話題。",
        ],
    );

    m.insert(
        "software_dev",
        vec![
            "Programming languages, frameworks, architecture, cloud/DevOps, and software engineering processes.",
            "プログラミング言語、フレームワーク、アーキテクチャ、クラウド/DevOps、運用や開発プロセスの話題。",
        ],
    );

    m.insert(
        "cybersecurity",
        vec![
            "Vulnerabilities, attacks/defense, incidents, authentication/cryptography, and security operations.",
            "脆弱性、攻撃・防御、インシデント、認証/暗号、セキュリティ運用の話題。",
        ],
    );

    m.insert(
        "consumer_tech",
        vec![
            "User-facing devices and services such as smartphones, PCs, wearables, and home electronics.",
            "スマホ、PC、ウェアラブル、家電など一般ユーザー向け製品やサービスの話題。",
        ],
    );

    m.insert(
        "internet_platforms",
        vec![
            "Trends and changes in online platforms such as social networks, search, streaming, and app stores.",
            "SNS、検索、動画配信、アプリストアなどオンラインプラットフォームの動向。",
        ],
    );

    m.insert(
        "space_astronomy",
        vec![
            "Space missions, satellites, exploration programs, and astronomy findings.",
            "宇宙開発、衛星、探査、天文学の観測成果に関する話題。",
        ],
    );

    m.insert(
        "climate_environment",
        vec![
            "Climate change, ecosystems, pollution control, and environmental protection trends.",
            "気候変動、生態系、汚染対策、環境保全の科学・社会動向。",
        ],
    );

    m.insert(
        "energy_transition",
        vec![
            "Renewables, grids, storage, nuclear, and decarbonization infrastructure and markets.",
            "再エネ、電力網、蓄電、原子力等のエネルギー供給と脱炭素インフラの動き。",
        ],
    );

    m.insert(
        "health_medicine",
        vec![
            "Healthcare systems, treatments, clinical topics, hospital operations, and public health impacts.",
            "医療制度、治療法、臨床、病院運営、感染症の社会的影響など。",
        ],
    );

    m.insert(
        "life_science",
        vec![
            "Biology, genetics, biotech, and foundational research results.",
            "生物学、遺伝子、バイオテック、基礎研究の成果や技術。",
        ],
    );

    m.insert(
        "economics_macro",
        vec![
            "Economy-wide trends: inflation, rates, employment, and international macro dynamics.",
            "景気、インフレ、金利、雇用、国際経済など経済全体の動き。",
        ],
    );

    m.insert(
        "markets_finance",
        vec![
            "Stocks, bonds, FX, fundraising, financial institutions, earnings and investment trends.",
            "株式、債券、為替、資金調達、金融機関、企業決算や投資動向。",
        ],
    );

    m.insert(
        "startups_innovation",
        vec![
            "Startups, venture capital, and innovation ecosystems.",
            "新興企業、VC、技術起点の新規事業やエコシステムの動き。",
        ],
    );

    m.insert(
        "industry_logistics",
        vec![
            "Manufacturing, supply chains, logistics networks and physical infrastructure operations.",
            "製造業、サプライチェーン、物流網、インフラ運用など実体経済の動き。",
        ],
    );

    m.insert(
        "politics_government",
        vec![
            "Elections, policymaking, public administration and political actors.",
            "選挙、政策決定、行政運営、政治家・政党の動向。",
        ],
    );

    m.insert(
        "diplomacy_security",
        vec![
            "International relations, conflicts, military affairs, treaties and security cooperation.",
            "国家間関係、国際紛争、軍事、条約、安全保障上の協力や緊張。",
        ],
    );

    m.insert(
        "law_crime",
        vec![
            "Courts, legal reforms, crime cases, and societal impacts of regulation.",
            "司法、法改正、裁判、犯罪事案、規制の社会的影響。",
        ],
    );

    m.insert(
        "education",
        vec![
            "Schooling, learning methods, education systems, and university operations.",
            "学校教育、学習方法、教育制度、大学運営などの話題。",
        ],
    );

    m.insert(
        "labor_workplace",
        vec![
            "Work styles, wages, HR systems, workplace safety, and culture changes.",
            "働き方、賃金、人事制度、労働安全、職場文化の変化。",
        ],
    );

    m.insert(
        "society_demographics",
        vec![
            "Population trends, regional issues, welfare, inequality and community change.",
            "人口動態、地域課題、福祉、格差、コミュニティの変化など。",
        ],
    );

    m.insert(
        "culture_arts",
        vec![
            "Fine arts, performing arts, traditional culture, exhibitions and cultural trends.",
            "美術、舞台、伝統文化、展覧会など文化・芸術の動向。",
        ],
    );

    m.insert(
        "film_tv",
        vec![
            "Films, drama, TV/streaming content, production and box-office/ratings trends.",
            "映画、ドラマ、テレビ/配信コンテンツ、制作・興行の動向。",
        ],
    );

    m.insert(
        "music_audio",
        vec![
            "Music releases, artists, live events, audio streaming and listening culture.",
            "音楽作品、アーティスト、ライブ、音声配信、オーディオ文化。",
        ],
    );

    m.insert(
        "sports",
        vec![
            "Sports competitions, leagues, athletes and tournaments.",
            "国内外の競技、リーグ運営、選手・大会の動向。",
        ],
    );

    m.insert(
        "food_cuisine",
        vec![
            "Food culture, dining trends, the food industry, recipes and culinary experiences.",
            "飲食トレンド、食文化、食品・外食産業の動き、レシピや食体験。",
        ],
    );

    m.insert(
        "travel_places",
        vec![
            "Tourism, destinations, travel experiences and demand shifts.",
            "観光、地域の魅力、旅行体験、旅行需要の変化。",
        ],
    );

    m.insert(
        "home_living",
        vec![
            "Housing, interior, home routines, and household services.",
            "住宅、インテリア、家事、暮らしの改善、家庭向けサービス。",
        ],
    );

    m.insert(
        "games_esports",
        vec![
            "Game releases, industry trends, esports tournaments, communities and business models.",
            "ゲームタイトル、業界動向、eスポーツ大会、コミュニティや収益モデル。",
        ],
    );

    m.insert(
        "mobility_automotive",
        vec![
            "Automotive, EVs, public transport, mobility technologies and markets.",
            "自動車、EV、公共交通、移動技術や市場の話題。",
        ],
    );

    m.insert(
        "consumer_products",
        vec![
            "Consumer goods choices, pricing, brands, household products and consumption trends.",
            "生活者向け商品の選択、価格動向、ブランド、日用品やサービスの消費傾向。",
        ],
    );

    m
});

#[allow(dead_code)] // May be used in future refactoring
pub fn get_canonical_sentences(genre: &str) -> Option<&'static Vec<&'static str>> {
    CANONICAL_SENTENCES.get(genre)
}
