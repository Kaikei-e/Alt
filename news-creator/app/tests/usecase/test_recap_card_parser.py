import pytest

from news_creator.usecase.recap_card_parser import (
    CardParseResult,
    parse_card_output,
    split_sentences,
)


def test_split_sentences():
    text = "ソラリス社はバージョン3.0を公開した。[1] メモリ使用量を40%削減した。[1][2]"
    sentences = split_sentences(text)
    assert len(sentences) == 2
    assert sentences[0] == "ソラリス社はバージョン3.0を公開した。[1]"
    assert sentences[1] == "メモリ使用量を40%削減した。[1][2]"


def test_split_sentences_newlines():
    text = "文1が発生した。[1]\n文2が続いた。[2]"
    sentences = split_sentences(text)
    assert len(sentences) == 2
    assert sentences[0] == "文1が発生した。[1]"
    assert sentences[1] == "文2が続いた。[2]"


def test_parse_happy_path():
    raw_text = """
【見出し】
ソラリス社、分散ログ基盤の次期版を公開
【何が起きた】
ソラリス社はログ収集エンジン「パルス」のバージョン3.0を正式公開した。[1]
メモリ使用量が従来比で40%削減され、毎秒10万件の転送に対応した。[1]
また、外部クラウドへの暗号化バックアップ機能が標準化された。[2]
【なぜ重要】
運用インフラのサーバ費用が年間約25%削減される。[1]
【出典】
[1] [2]
"""
    result: CardParseResult = parse_card_output(raw_text, valid_refs={1, 2})
    assert result.success is True
    assert result.card is not None
    assert result.card.headline_ja == "ソラリス社、分散ログ基盤の次期版を公開"
    assert len(result.card.what_ja) == 3
    assert result.card.what_ja[0].refs == [1]
    assert result.card.what_ja[2].refs == [2]
    assert result.card.why_ja is not None
    assert result.card.why_ja.refs == [1]
    assert result.card.used_refs == [1, 2]


def test_parse_why_ja_none_on_gaitou_nashi():
    raw_text = """
【見出し】
ソラリス社、分散ログ基盤の次期版を公開
【何が起きた】
ソラリス社はログ収集エンジン「パルス」のバージョン3.0を正式公開した。[1]
メモリ使用量が従来比で40%削減された。[1]
【なぜ重要】
該当なし
【出典】
[1]
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is True
    assert result.card is not None
    assert result.card.why_ja is None
    assert result.card.used_refs == [1]


def test_parse_multiple_refs_per_sentence():
    raw_text = """
【見出し】
複数出典のテスト見出し
【何が起きた】
複数社の共同開発により新規格が策定された。[1][2]
相互運用性テストが完了した。[2]
【なぜ重要】
該当なし
【出典】
[1] [2]
"""
    result = parse_card_output(raw_text, valid_refs={1, 2})
    assert result.success is True
    assert result.card is not None
    assert result.card.what_ja[0].refs == [1, 2]
    assert result.card.used_refs == [1, 2]


def test_parse_missing_tag():
    raw_text = """
【何が起きた】
見出しタグが存在しないテキスト。[1]
二文目の記述。[1]
【なぜ重要】
該当なし
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "missing_tag"


def test_parse_empty_output():
    result = parse_card_output("   \n\t  ", valid_refs={1})
    assert result.success is False
    assert result.reason == "empty_output"
    assert result.detail is None


def test_parse_unknown_ref():
    raw_text = """
【見出し】
未知の参照テスト
【何が起きた】
ソラリス社が新機能を発表した。[1]
未知の出典番号を参照している文。[99]
【なぜ重要】
該当なし
【出典】
[1] [99]
"""
    result = parse_card_output(raw_text, valid_refs={1, 2})
    assert result.success is False
    assert result.reason == "unknown_ref"
    assert result.detail is None


def test_parse_sentence_count_bounds():
    # 1 sentence in what_ja (< 2)
    raw_text_1_sentence = """
【見出し】
一文だけのテスト見出し
【何が起きた】
一文目だけです。[1]
【なぜ重要】
該当なし
"""
    result_1 = parse_card_output(raw_text_1_sentence, valid_refs={1})
    assert result_1.success is False
    assert result_1.reason == "parse_failed"
    assert result_1.detail == "sentence_count"

    # 4 sentences in what_ja (> 3)
    raw_text_4_sentences = """
【見出し】
四文のテスト見出し
【何が起きた】
一文目です。[1]
二文目です。[1]
三文目です。[1]
四文目です。[1]
【なぜ重要】
該当なし
"""
    result = parse_card_output(raw_text_4_sentences, valid_refs={1})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "sentence_count"

    # 2 sentences in why_ja (> 1)
    raw_text_2_why = """
【見出し】
なぜ重要二文テスト
【何が起きた】
一文目です。[1]
二文目です。[1]
【なぜ重要】
重要理由一文目。[1]
重要理由二文目。[1]
"""
    result2 = parse_card_output(raw_text_2_why, valid_refs={1})
    assert result2.success is False
    assert result2.reason == "parse_failed"
    assert result2.detail == "why_format"


def test_parse_sentence_missing_ref():
    raw_text = """
【見出し】
参照欠落テスト
【何が起きた】
一文目に参照がありません。
二文目には参照があります。[1]
【なぜ重要】
該当なし
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "missing_citation"


def test_parse_language_gate():
    raw_text = """
【見出し】
English Only Headline For Test
【何が起きた】
This is the first English sentence without Japanese chars.[1]
This is the second English sentence without Japanese chars.[1]
【なぜ重要】
該当なし
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "language"


def test_parse_headline_bounds():
    # 60 chars is the max allowed length
    valid_headline = "あ" * 60
    raw_text_valid = f"""
【見出し】
{valid_headline}
【何が起きた】
一文目です。[1]
二文目です。[1]
【なぜ重要】
該当なし
"""
    result_valid = parse_card_output(raw_text_valid, valid_refs={1})
    assert result_valid.success is True
    assert result_valid.card is not None
    assert len(result_valid.card.headline_ja) == 60

    # 61 chars exceeds the limit
    long_headline = "あ" * 61
    raw_text = f"""
【見出し】
{long_headline}
【何が起きた】
一文目です。[1]
二文目です。[1]
【なぜ重要】
該当なし
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "headline_too_long"


def test_parse_strips_thinking_blocks_and_turn_tokens():
    raw_text = """<|channel>thought
Here is some inner thinking that should be stripped
<channel|>
<start_of_turn>model
【見出し】
思考ブロック除去テスト
【何が起きた】
思考ブロックが正常に除去されてパースされる。[1]
二文目の内容です。[1]
【なぜ重要】
該当なし
<end_of_turn>
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is True
    assert result.card is not None
    assert result.card.headline_ja == "思考ブロック除去テスト"


def test_clean_raw_output_reuses_neutralize_control_tokens():
    from news_creator.usecase.recap_card_parser import clean_raw_output

    nested = "<|tur<|turn>n>テスト"
    cleaned = clean_raw_output(nested)
    assert "<|turn>" not in cleaned
    assert "テスト" in cleaned


def test_parse_why_ja_startswith_gaitou_nashi():
    raw_text = """
【見出し】
プレフィックス判定テスト
【何が起きた】
一文目です。[1]
二文目です。[1]
【なぜ重要】
該当なし（特段の影響は報告されていない）
【出典】
[1]
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is True
    assert result.card is not None
    assert result.card.why_ja is None


def test_clean_raw_output_strips_hidden_and_bidi_characters():
    # Headline and body containing zero-width space (U+200B), BOM (U+FEFF), and BiDi override (U+202E)
    raw_text = (
        "【見出し】\n"
        "隠し\u200b文字\ufeff除去\u202eテスト\n"
        "【何が起きた】\n"
        "本文の\u200b隠し文字も\ufeff除去される。[1]\n"
        "二文目の記述です。[1]\n"
        "【なぜ重要】\n"
        "該当なし\n"
        "【出典】\n"
        "[1]\n"
    )
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is True
    assert result.card is not None
    assert result.card.headline_ja == "隠し文字除去テスト"
    assert "\u200b" not in result.card.headline_ja
    assert "\ufeff" not in result.card.headline_ja
    assert "\u202e" not in result.card.headline_ja
    assert "\u200b" not in result.card.what_ja[0].text


@pytest.mark.parametrize(
    "raw_text",
    [
        # Leading ```markdown and trailing fence
        """```markdown
【見出し】
コードフェンス付き出力テスト
【何が起きた】
LLMがコードフェンスで囲んで出力した場合でも正常にパースされる。[1]
二文目も正常に認識される。[1]
【なぜ重要】
運用の効率化に大きく寄与する。[1]
```""",
        # Leading ```text and trailing fence
        """```text
【見出し】
コードフェンス付き出力テスト
【何が起きた】
LLMがコードフェンスで囲んで出力した場合でも正常にパースされる。[1]
二文目も正常に認識される。[1]
【なぜ重要】
運用の効率化に大きく寄与する。[1]
```""",
        # Fence without a trailing newline after opening fence
        "```【見出し】\nコードフェンス付き出力テスト\n【何が起きた】\nLLMがコードフェンスで囲んで出力した場合でも正常にパースされる。[1]\n二文目も正常に認識される。[1]\n【なぜ重要】\n運用の効率化に大きく寄与する。[1]\n```",
        # Trailing fence alone without leading fence
        """【見出し】
コードフェンス付き出力テスト
【何が起きた】
LLMがコードフェンスで囲んで出力した場合でも正常にパースされる。[1]
二文目も正常に認識される。[1]
【なぜ重要】
運用の効率化に大きく寄与する。[1]
```""",
    ],
)
def test_parse_strips_markdown_code_fences(raw_text: str):
    """Verify parser strips leading/trailing markdown code fences with/without lang tags."""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is True
    assert result.card is not None
    assert result.card.headline_ja == "コードフェンス付き出力テスト"
    assert len(result.card.what_ja) == 2
    assert result.card.why_ja is not None
    assert result.card.why_ja.refs == [1]


def test_parse_accepts_citations_before_or_after_punctuation():
    """Verify citations placed either before or after terminal punctuation are accepted."""
    raw_text = """
【見出し】
文末引用位置の柔軟性テスト
【何が起きた】
第一文は句点の後に引用を配置した。[1]
第二文は句点の前に引用を配置した[1]。
第三文は句点の前に複数引用を配置した[1]　[2]。
【なぜ重要】
影響の記述でも句点の前に引用を配置できる[1]。
【出典】
[1] [2]
"""
    result = parse_card_output(raw_text, valid_refs={1, 2})
    assert result.success is True
    assert result.card is not None
    assert len(result.card.what_ja) == 3
    assert result.card.what_ja[0].refs == [1]
    assert result.card.what_ja[1].refs == [1]
    assert result.card.what_ja[2].refs == [1, 2]
    assert result.card.why_ja is not None
    assert result.card.why_ja.refs == [1]


def test_parse_normalizes_full_width_brackets():
    """Verify full-width brackets ［1］ are normalized to [1] and accepted."""
    raw_text = """
【見出し】
全角ブラケット正規化テスト
【何が起きた】
全角ブラケットで引用番号が出力された。［1］
二文目の全角ブラケットも正規化される［1］。
【なぜ重要】
該当なし
【出典】
［1］
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is True
    assert result.card is not None
    assert result.card.headline_ja == "全角ブラケット正規化テスト"
    assert result.card.what_ja[0].refs == [1]
    assert result.card.what_ja[1].refs == [1]
    assert result.card.used_refs == [1]


def test_parse_language_gate_records_ratio_and_counts():
    """Verify language gate rejection records measured_ratio and character_counts."""
    raw_text = """
【見出し】
Snowflake Tokyo 2026 LayerX発表
【何が起きた】
Snowflake World Tour Tokyo 2026 is an enterprise event.[1]
LayerX TechHarmony architecture was showcased today.[1]
【なぜ重要】
該当なし
【出典】
[1]
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "language"
    assert result.measured_ratio is not None
    assert isinstance(result.measured_ratio, float)
    assert result.measured_ratio < 0.6
    assert result.character_counts is not None
    assert "japanese" in result.character_counts
    assert "substantive" in result.character_counts
    assert "total" in result.character_counts
    assert "stripped" in result.character_counts
    assert result.character_counts["japanese"] >= 0
    assert result.character_counts["total"] > 0


def test_parse_four_sentences_rejected():
    raw_text = """【見出し】
DroidKaigi 2026への各企業の参加と活動報告
【何が起きた】
STORESはスポンサーとしてDroidKaigi 2026に現地で参加した[1]。エブリーはゴールドスポンサーとしてブースを出展し、イベントの様子を紹介した[2]。TVerはブースで技術展示を行った[3]。TRUSTDOCKもDroidKaigi 2026への参加レポートを公開した[4]。
【なぜ重要】
該当なし
【出典】
[1] [2] [3] [4]"""
    result = parse_card_output(raw_text, valid_refs={1, 2, 3, 4})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "sentence_count"


def test_parse_bullet_separator():
    raw_text = """【見出し】
Logitechが開発者向けAI操作キーパッドMX Keypadを発売
【何が起きた】
Logitechは、コーディングやAIワークフロー向けのアクセサリ「MX Keypad」を発表した[1]・[2]。この製品は9つのカスタマイズ可能なキーとタッチスクリーンを搭載し、価格は99.99ドルである[1]・[2]。
【なぜ重要】
該当なし
【出典】
[1] [2]"""
    result = parse_card_output(raw_text, valid_refs={1, 2})
    assert result.success is True
    assert result.card is not None


def test_parse_long_headline_with_latin_names():
    raw_text = """【見出し】
Tschabalala Selfの「Lady in Blue」がトラファルガー広場の第四台座に登場
【何が起きた】
Tschabalala Selfによる青い女性像である「Lady in Blue」がロンドンのトラファルガー広場にある第四の台座に設置された[1] [2]。この作品は現代のロンドンを歩く若い有色人種の女性に敬意を表したブロンズ彫刻である[1]。
【なぜ重要】
該当なし
【出典】
[1] [2]"""
    result = parse_card_output(raw_text, valid_refs={1, 2})
    assert result.success is True
    assert result.card is not None


def test_parse_bracket_comma_and_oyobi():
    raw_text = """【見出し】
Apple、新型AirPods 5を発表しノイズキャンセリングを強化
【何が起きた】
AppleはiPhone発表イベントで次世代の完全ワイヤレスイヤホン「AirPods 5」を発表した[2]および[6]。この新モデルは業界最高クラスのアクティブノイズキャンセリング（ANC）を搭載していると謳われている[2, 6]。また、AirPods 5は現在Amazonなどで予約可能になっている[3]。
【なぜ重要】
新型のApple Watch Ultra 4やSeries 12などと共に、一部製品で割引価格での購入が可能である[1] [4]。
【出典】
[1] [2] [3] [4] [6]"""
    result = parse_card_output(raw_text, valid_refs={1, 2, 3, 4, 5, 6})
    assert result.success is True
    assert result.card is not None


def test_parse_citation_then_touten():
    raw_text = """【見出し】
Go Conference 2026の開催概要と参加レポート
【何が起きた】
Go Conference 2026は2026年9月11日に中野セントラルパークカンファレンスで開催された[4]、登壇者がいる[2]。クロージング発表によると、会場とオンラインを合わせて700人超の参加者があった[3]。プロポーザルには約175件応募があり、採択率は15.9倍であったことが示されている[3]。
【なぜ重要】
該当なし
【出典】
[2] [3] [4]"""
    result = parse_card_output(raw_text, valid_refs={1, 2, 3, 4})
    assert result.success is True
    assert result.card is not None


def test_parse_unknown_citation_format_detail():
    raw_text = """【見出し】
引用形式不正テスト
【何が起きた】
一文目です。[1]
二文目は[1]、新機能が追加されたと発表された。
【なぜ重要】
該当なし
【出典】
[1]"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "unknown_citation_format"


def test_parse_sources_tag_detail():
    raw_text = """【見出し】
出典タグ空テスト
【何が起きた】
一文目です。[1]
二文目です。[1]
【なぜ重要】
該当なし
【出典】
"""
    result = parse_card_output(raw_text, valid_refs={1})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "sources_tag"


def test_strip_source_latin_runs_preserves_ten_token_lede_with_commas_and_strips_brand():
    from news_creator.usecase.recap_card_parser import strip_source_latin_runs

    ten_token_lede = "According to recent industry reports, major tech companies announced significant updates yesterday."
    source_texts = [
        ten_token_lede,
        "Samsung Galaxy Z Fold8: next-generation foldable device.",
    ]
    # The 10-token lede sentence copied verbatim with commas is not stripped even partially
    assert strip_source_latin_runs(ten_token_lede, source_texts) == ten_token_lede

    # Samsung Galaxy Z Fold8 is an allowed run (<= 4 tokens) and is stripped
    brand_text = "Samsung Galaxy Z Fold8"
    assert strip_source_latin_runs(brand_text, source_texts) == ""


def test_strip_source_latin_runs_adjacent_to_kana():
    from news_creator.usecase.recap_card_parser import strip_source_latin_runs

    source_texts = [
        "iPhone Duo: specifications and features.",
        "Node.js: modern JavaScript runtime.",
    ]
    # iPhone Duo and Node.js immediately adjacent to Japanese kana
    card_snippet = "iPhone DuoとNode.jsの連携"
    assert strip_source_latin_runs(card_snippet, source_texts) == "との連携"


def test_parse_latin_names_in_source_stripped_before_language_check():
    """Verify Latin product names occurring in source items are stripped before measuring Japanese ratio."""
    raw_text = """【見出し】
iPhone DuoとSamsung Galaxy Z Fold8の比較検証
【何が起きた】
「iPhone Duo」が公式に発表され、そのハードウェア仕様が同サイズのSamsung Galaxy Z Fold8と比較されている[1] [2]。本体サイズはiPhone Duoの方が薄型だが、重量はGalaxyの方が軽い設計となっている[1]。また、両機種ともに独自の折りたたみヒンジ技術を採用している[2]。
【なぜ重要】
該当なし
【出典】
[1] [2]"""
    source_texts = [
        "iPhone Duo, Samsung Galaxy Z Fold8, Galaxy: official hardware comparison.",
        "Hardware specs: iPhone Duo is thinner, Galaxy is lighter with folding hinge.",
    ]
    # Without source_texts, the card fails language gate because Latin names drag ratio under 0.6
    res_without_source = parse_card_output(raw_text, valid_refs={1, 2})
    assert res_without_source.success is False
    assert res_without_source.reason == "language"

    # With source_texts, the Latin names from source are stripped before ratio check and it passes
    res_with_source = parse_card_output(
        raw_text, valid_refs={1, 2}, source_texts=source_texts
    )
    assert res_with_source.success is True
    assert res_with_source.card is not None
    assert (
        res_with_source.card.headline_ja
        == "iPhone DuoとSamsung Galaxy Z Fold8の比較検証"
    )


def test_parse_unrelated_latin_prose_still_fails_language_check_with_sources():
    """Verify unrelated Latin prose still fails language check even when source items are provided."""
    raw_text = """【見出し】
Tech News Weekly Update
【何が起きた】
This is completely unrelated English prose that was generated by the LLM.[1]
Another sentence written purely in Latin script describing events.[1]
【なぜ重要】
該当なし
【出典】
[1]"""
    source_texts = [
        "Tech News Weekly Update",
        "Some Japanese source text or unrelated content.",
    ]
    res = parse_card_output(raw_text, valid_refs={1}, source_texts=source_texts)
    assert res.success is False
    assert res.reason == "language"


def test_parse_strips_citation_markers_from_headline():
    raw_text = """【見出し】
AppleがiPhone 18シリーズを発表：折りたたみモデルや可変絞りカメラ搭載 [1] [3]
【何が起きた】
Appleは新製品発表会で次世代スマートフォン「iPhone 18」シリーズを正式に発表した[1]。折りたたみモデルの追加や可変絞りカメラの搭載などハードウェアの刷新が明らかになった[3]。
【なぜ重要】
該当なし
【出典】
[1] [3]"""
    result = parse_card_output(raw_text, valid_refs={1, 3})
    assert result.success is True
    assert result.card is not None
    assert (
        result.card.headline_ja
        == "AppleがiPhone 18シリーズを発表：折りたたみモデルや可変絞りカメラ搭載"
    )


def test_parse_headline_only_citations_fails_as_missing_tag():
    raw_text = """【見出し】
[1] [3]
【何が起きた】
Appleは新製品発表会で次世代スマートフォンを発表した[1]。折りたたみモデルが追加された[3]。
【なぜ重要】
該当なし
【出典】
[1] [3]"""
    result = parse_card_output(raw_text, valid_refs={1, 3})
    assert result.success is False
    assert result.reason == "parse_failed"
    assert result.detail == "missing_tag"
