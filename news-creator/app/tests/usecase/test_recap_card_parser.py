"""Tests for recap card output tag parser."""

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


def test_parse_empty_output():
    result = parse_card_output("   \n\t  ", valid_refs={1})
    assert result.success is False
    assert result.reason == "empty_output"


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
    long_headline = "あ" * 41
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
