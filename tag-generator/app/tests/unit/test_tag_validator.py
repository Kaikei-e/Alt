"""
Unit tests for shared tag_validator module.

Tests the is_valid_japanese_tag and clean_noun_phrase functions that provide
unified tag validation across all extractors.
"""

import pytest


class TestIsValidJapaneseTag:
    """Tests for is_valid_japanese_tag function."""

    @pytest.fixture
    def validator(self):
        """Import the validator function."""
        from tag_extractor.tag_validator import is_valid_japanese_tag

        return is_valid_japanese_tag

    # Length validation tests
    def test_rejects_empty_tag(self, validator):
        """Empty tags should be rejected."""
        assert validator("") is False

    def test_rejects_single_char_tag(self, validator):
        """Single character tags should be rejected."""
        assert validator("A") is False
        assert validator("あ") is False

    def test_accepts_two_char_tag(self, validator):
        """Two character tags should be accepted."""
        assert validator("AI") is True
        assert validator("機械") is True

    def test_accepts_normal_length_tag(self, validator):
        """Normal length tags (2-15 chars) should be accepted."""
        assert validator("機械学習") is True
        assert validator("TensorFlow") is True
        assert validator("セキュリティ") is True

    def test_accepts_exactly_15_char_tag(self, validator):
        """Tags exactly 15 characters should be accepted."""
        tag_15_chars = "あ" * 15
        assert validator(tag_15_chars) is True

    def test_rejects_tag_over_15_chars(self, validator):
        """Tags over 15 characters should be rejected by default."""
        tag_16_chars = "あ" * 16
        assert validator(tag_16_chars) is False

    def test_custom_max_length(self, validator):
        """Custom max_length parameter should be respected."""
        tag_20_chars = "あ" * 20
        assert validator(tag_20_chars, max_length=20) is True
        assert validator(tag_20_chars, max_length=15) is False

    def test_rejects_sentence_fragment(self, validator):
        """Sentence-like fragments should be rejected."""
        # These are real examples from the issue
        assert validator("Databricksのセキュリティは") is False
        assert validator("TablesはDatabricksが管理する") is False

    # Verb/auxiliary verb ending tests - comprehensive
    def test_rejects_desu_ending(self, validator):
        """Tags ending with です should be rejected."""
        assert validator("便利です") is False
        assert validator("機械学習です") is False

    def test_rejects_masu_ending(self, validator):
        """Tags ending with ます should be rejected."""
        assert validator("使います") is False
        assert validator("動作します") is False

    def test_rejects_mashita_ending(self, validator):
        """Tags ending with ました should be rejected."""
        assert validator("完了しました") is False
        assert validator("実装しました") is False

    def test_rejects_teiru_ending(self, validator):
        """Tags ending with ている should be rejected."""
        assert validator("動いている") is False
        assert validator("使用している") is False

    def test_rejects_shita_ending(self, validator):
        """Tags ending with した should be rejected."""
        assert validator("実装した") is False
        assert validator("開発した") is False

    def test_rejects_suru_ending(self, validator):
        """Tags ending with する should be rejected."""
        assert validator("実行する") is False
        assert validator("処理する") is False

    def test_rejects_nai_ending(self, validator):
        """Tags ending with ない should be rejected."""
        assert validator("できない") is False
        assert validator("動作しない") is False

    def test_rejects_aru_ending(self, validator):
        """Tags ending with ある should be rejected."""
        assert validator("必要がある") is False

    def test_rejects_iru_ending(self, validator):
        """Tags ending with いる should be rejected."""
        assert validator("使っている") is False

    def test_rejects_reru_ending(self, validator):
        """Tags ending with れる should be rejected."""
        assert validator("呼ばれる") is False
        assert validator("使用される") is False

    def test_rejects_rareru_ending(self, validator):
        """Tags ending with られる should be rejected."""
        assert validator("考えられる") is False

    def test_rejects_imasu_ending(self, validator):
        """Tags ending with います should be rejected."""
        assert validator("動いています") is False
        assert validator("使っています") is False

    def test_rejects_teimasu_ending(self, validator):
        """Tags ending with ています should be rejected."""
        assert validator("動作しています") is False

    def test_rejects_shou_ending(self, validator):
        """Tags ending with しょう should be rejected."""
        assert validator("使いましょう") is False

    def test_rejects_deshou_ending(self, validator):
        """Tags ending with でしょう should be rejected."""
        assert validator("便利でしょう") is False

    # Particle ending tests - ALL lengths (key change from original)
    def test_rejects_particle_ha_ending(self, validator):
        """Tags ending with は should be rejected regardless of length."""
        assert validator("これは") is False
        assert validator("Databricksは") is False
        assert validator("セキュリティは") is False

    def test_rejects_particle_ga_ending(self, validator):
        """Tags ending with が should be rejected regardless of length."""
        assert validator("これが") is False
        assert validator("機械学習が") is False
        assert validator("セキュリティが") is False

    def test_rejects_particle_wo_ending(self, validator):
        """Tags ending with を should be rejected regardless of length."""
        assert validator("これを") is False
        assert validator("データを") is False

    def test_rejects_particle_ni_ending(self, validator):
        """Tags ending with に should be rejected regardless of length."""
        assert validator("ここに") is False
        assert validator("サーバーに") is False

    def test_rejects_particle_de_ending(self, validator):
        """Tags ending with で should be rejected regardless of length."""
        assert validator("ここで") is False
        assert validator("クラウドで") is False

    def test_rejects_particle_to_ending(self, validator):
        """Tags ending with と should be rejected regardless of length."""
        assert validator("AIと") is False
        assert validator("セキュリティと") is False

    def test_rejects_particle_no_ending(self, validator):
        """Tags ending with の (particle) should be rejected."""
        # Note: の at the end is almost always a particle in natural text
        assert validator("これの") is False
        assert validator("Databricksの") is False

    def test_rejects_particle_he_ending(self, validator):
        """Tags ending with へ should be rejected."""
        assert validator("こちらへ") is False

    def test_rejects_particle_ya_ending(self, validator):
        """Tags ending with や should be rejected."""
        assert validator("AWSや") is False

    def test_rejects_particle_mo_ending(self, validator):
        """Tags ending with も should be rejected."""
        assert validator("これも") is False
        assert validator("APIも") is False

    def test_rejects_particle_ka_ending(self, validator):
        """Tags ending with か should be rejected."""
        assert validator("何か") is False

    def test_rejects_particle_na_ending(self, validator):
        """Tags ending with な (adjectival particle) should be rejected."""
        assert validator("便利な") is False
        assert validator("重要な") is False

    # Number-only tests
    def test_rejects_number_only(self, validator):
        """Tags that are numbers only should be rejected."""
        assert validator("2025") is False
        assert validator("12") is False
        assert validator("100") is False
        assert validator("0") is False

    def test_accepts_number_with_text(self, validator):
        """Tags with numbers and text should be accepted."""
        assert validator("Web3") is True
        assert validator("5G通信") is True
        assert validator("3Dプリンター") is True
        assert validator("iOS17") is True

    # URL/HTML fragment tests
    def test_rejects_https_fragment(self, validator):
        """Tags that are 'https' should be rejected."""
        assert validator("https") is False
        assert validator("HTTPS") is False
        assert validator("http") is False

    def test_rejects_www_fragment(self, validator):
        """Tags that are 'www' should be rejected."""
        assert validator("www") is False
        assert validator("WWW") is False

    def test_rejects_domain_fragment(self, validator):
        """Tags that are domain TLDs should be rejected."""
        assert validator("com") is False
        assert validator("org") is False
        assert validator("net") is False
        assert validator("html") is False

    def test_rejects_html_entity_fragments(self, validator):
        """Tags that are HTML entity fragments should be rejected."""
        assert validator("gt") is False
        assert validator("lt") is False
        assert validator("amp") is False
        assert validator("nbsp") is False

    # Valid tag tests (positive cases)
    def test_accepts_valid_tech_terms(self, validator):
        """Valid tech terms should be accepted."""
        valid_tags = [
            "機械学習",
            "TensorFlow",
            "GitHub",
            "AWS",
            "API",
            "Python",
            "JavaScript",
            "クラウド",
            "セキュリティ",
            "Databricks",
            "Unity Catalog",
            "データガバナンス",
        ]
        for tag in valid_tags:
            assert validator(tag) is True, f"Tag '{tag}' should be valid"

    def test_accepts_valid_japanese_nouns(self, validator):
        """Valid Japanese nouns should be accepted."""
        valid_tags = [
            "技術",
            "開発",
            "データベース",
            "アルゴリズム",
            "ネットワーク",
            "サーバー",
        ]
        for tag in valid_tags:
            assert validator(tag) is True, f"Tag '{tag}' should be valid"


class TestCleanNounPhrase:
    """Tests for clean_noun_phrase function."""

    @pytest.fixture
    def cleaner(self):
        """Import the cleaner function."""
        from tag_extractor.tag_validator import clean_noun_phrase

        return clean_noun_phrase

    # Particle removal tests
    def test_removes_trailing_ha(self, cleaner):
        """Should remove trailing は."""
        assert cleaner("セキュリティは") == "セキュリティ"
        assert cleaner("Databricksは") == "Databricks"

    def test_removes_trailing_ga(self, cleaner):
        """Should remove trailing が."""
        assert cleaner("セキュリティが") == "セキュリティ"

    def test_removes_trailing_wo(self, cleaner):
        """Should remove trailing を."""
        assert cleaner("データを") == "データ"

    def test_removes_trailing_ni(self, cleaner):
        """Should remove trailing に."""
        assert cleaner("サーバーに") == "サーバー"

    def test_removes_trailing_de(self, cleaner):
        """Should remove trailing で."""
        assert cleaner("クラウドで") == "クラウド"

    def test_removes_trailing_no(self, cleaner):
        """Should remove trailing の."""
        assert cleaner("Databricksの") == "Databricks"

    # Verb ending removal tests
    def test_removes_trailing_desu(self, cleaner):
        """Should remove trailing です."""
        assert cleaner("便利です") == "便利"

    def test_removes_trailing_masu(self, cleaner):
        """Should remove trailing ます."""
        # Note: います is matched as a whole, leaving 使
        assert cleaner("使います") == "使"

    def test_removes_trailing_mashita(self, cleaner):
        """Should remove trailing ました."""
        assert cleaner("完了しました") == "完了し"

    def test_removes_trailing_teiru(self, cleaner):
        """Should remove trailing ている."""
        assert cleaner("動いている") == "動い"

    def test_removes_trailing_suru(self, cleaner):
        """Should remove trailing する."""
        assert cleaner("実行する") == "実行"

    def test_removes_trailing_imasu(self, cleaner):
        """Should remove trailing います."""
        # Note: ています is matched as a whole, then particle doesn't match
        assert cleaner("動いています") == "動い"

    def test_removes_trailing_teimasu(self, cleaner):
        """Should remove trailing ています."""
        assert cleaner("動作しています") == "動作し"

    # No change tests
    def test_leaves_valid_nouns_unchanged(self, cleaner):
        """Valid nouns should not be modified."""
        assert cleaner("セキュリティ") == "セキュリティ"
        assert cleaner("Databricks") == "Databricks"
        assert cleaner("機械学習") == "機械学習"
        assert cleaner("Unity Catalog") == "Unity Catalog"

    def test_handles_empty_string(self, cleaner):
        """Should handle empty string."""
        assert cleaner("") == ""

    def test_strips_whitespace(self, cleaner):
        """Should strip whitespace."""
        assert cleaner("  セキュリティ  ") == "セキュリティ"


class TestRealWorldExamples:
    """Tests based on real problematic tags from the issue."""

    @pytest.fixture
    def validator(self):
        """Import the validator function."""
        from tag_extractor.tag_validator import is_valid_japanese_tag

        return is_valid_japanese_tag

    @pytest.fixture
    def cleaner(self):
        """Import the cleaner function."""
        from tag_extractor.tag_validator import clean_noun_phrase

        return clean_noun_phrase

    def test_databricks_sentence_fragments_rejected(self, validator):
        """Real problematic tags from the issue should be rejected."""
        problematic_tags = [
            "Databricksのセキュリティは",
            "Databricks運用の重要ポイントを網羅する内容になっています",
            "TablesはDatabricksが管理する",
            "がセキュリティ設計の鉄則です",
        ]
        for tag in problematic_tags:
            assert validator(tag) is False, f"Tag '{tag}' should be rejected"

    def test_expected_databricks_tags_accepted(self, validator):
        """Expected proper tags for Databricks articles should be accepted."""
        expected_tags = [
            "Databricks",
            "セキュリティ",
            "Unity Catalog",
            "データガバナンス",
            "データレイク",
        ]
        for tag in expected_tags:
            assert validator(tag) is True, f"Tag '{tag}' should be accepted"

    def test_cleaner_fixes_common_patterns(self, cleaner):
        """Cleaner should fix common problematic patterns."""
        # Particle endings
        assert cleaner("Databricksのセキュリティは") == "Databricksのセキュリティ"
        assert cleaner("Unity Catalogで") == "Unity Catalog"

    def test_combined_validation_and_cleaning(self, validator, cleaner):
        """Cleaning followed by validation should work for recoverable cases."""
        # After cleaning, some tags become valid
        cleaned = cleaner("セキュリティは")
        assert cleaned == "セキュリティ"
        assert validator(cleaned) is True

        # Some are still invalid after cleaning (too short, etc.)
        cleaned = cleaner("これは")
        assert cleaned == "これ"
        # "これ" is 2 chars, which is valid
        assert validator(cleaned) is True
