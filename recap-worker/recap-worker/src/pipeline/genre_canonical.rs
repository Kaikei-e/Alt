use std::collections::HashMap;

use once_cell::sync::Lazy;

/// Canonical sentences for each genre to be used for embedding-based filtering.
/// These sentences represent the "ideal" content for a genre.
static CANONICAL_SENTENCES: Lazy<HashMap<&'static str, Vec<&'static str>>> = Lazy::new(|| {
    let mut m = HashMap::new();

    m.insert(
        "politics",
        vec![
            "The government passed a new bill regarding tax reform.",
            "The president announced a new policy on foreign relations.",
            "Parliamentary elections are scheduled for next month.",
            "Political parties are debating the new legislation.",
            "The prime minister resigned after the scandal.",
            "政府は新しい税制改革法案を可決しました。",
            "大統領は外交関係に関する新しい方針を発表しました。",
            "議会選挙は来月予定されています。",
            "政党は新しい法律について議論しています。",
            "首相はスキャンダルの後に辞任しました。",
        ],
    );

    m.insert(
        "business",
        vec![
            "The company reported a significant increase in quarterly revenue.",
            "Stock markets rallied after the positive economic data.",
            "A major merger between two tech giants was announced.",
            "Inflation rates have slightly decreased this month.",
            "The central bank decided to raise interest rates.",
            "その会社は四半期収益の大幅な増加を報告しました。",
            "株式市場は肯定的な経済データの後に上昇しました。",
            "2つのハイテク巨人間の主要な合併が発表されました。",
            "インフレ率は今月わずかに減少しました。",
            "中央銀行は金利を引き上げることを決定しました。",
        ],
    );

    m.insert(
        "tech",
        vec![
            "A new smartphone with advanced AI features was released.",
            "Software developers are adopting the new programming language.",
            "Cybersecurity threats are increasing with the rise of IoT.",
            "Cloud computing services are expanding their infrastructure.",
            "The startup raised funding for its innovative app.",
            "高度なAI機能を備えた新しいスマートフォンが発売されました。",
            "ソフトウェア開発者は新しいプログラミング言語を採用しています。",
            "IoTの台頭に伴い、サイバーセキュリティの脅威が増加しています。",
            "クラウドコンピューティングサービスはインフラを拡大しています。",
            "そのスタートアップは革新的なアプリのために資金を調達しました。",
        ],
    );

    m.insert(
        "ai",
        vec![
            "Researchers developed a new large language model.",
            "Artificial intelligence is transforming the healthcare industry.",
            "Deep learning algorithms are improving image recognition.",
            "Generative AI tools are becoming more accessible.",
            "The ethics of AI development are being discussed.",
            "研究者は新しい大規模言語モデルを開発しました。",
            "人工知能はヘルスケア業界を変革しています。",
            "ディープラーニングアルゴリズムは画像認識を向上させています。",
            "生成AIツールはよりアクセスしやすくなっています。",
            "AI開発の倫理が議論されています。",
        ],
    );

    // Add more genres as needed...
    // For now, we focus on the ones that had issues (Politics, Business) and major ones.

    m
});

pub fn get_canonical_sentences(genre: &str) -> Option<&'static Vec<&'static str>> {
    CANONICAL_SENTENCES.get(genre)
}
