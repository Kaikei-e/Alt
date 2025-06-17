module_name = "tag_extractor"

from sentence_transformers import SentenceTransformer
from keybert import KeyBERT
from langdetect import detect
from sentence_transformers import SentenceTransformer
from fugashi import Tagger
import nltk, unicodedata, re
import os


# Load models once at import time - force CPU to avoid GPU memory issues
print("Loading SentenceTransformer model...")
embedder = SentenceTransformer("paraphrase-multilingual-MiniLM-L12-v2", device='cpu')
kw = KeyBERT(embedder)
ja_tagger = Tagger("-Owakati")  # UniDicを事前インストール
print("Models loaded successfully")

# Get the directory of this file and construct paths to stopword files
current_dir = os.path.dirname(__file__)
ja_stopwords_path = os.path.join(current_dir, "stopwords_ja.txt")
en_stopwords_path = os.path.join(current_dir, "stopwords_en.txt")

# Load custom stopwords - fix the line-by-line reading
with open(ja_stopwords_path, 'r', encoding='utf-8') as f:
    ja_stop = set(line.strip() for line in f if line.strip())

with open(en_stopwords_path, 'r', encoding='utf-8') as f:
    en_stop = set(line.strip().lower() for line in f if line.strip())

# Combine with NLTK English stopwords for comprehensive coverage
try:
    en_stop.update(set(nltk.corpus.stopwords.words("english")))
except:
    print("Warning: NLTK English stopwords not available")

def normalize_ja(t): return unicodedata.normalize("NFKC", t)
def normalize_en(t): return t.lower()

def candidate_tokens(text: str, lang: str) -> list[str]:
    "Tokenize + stop-word removal (lang‐specific)"
    if lang == "ja":
        toks = [w.surface for w in ja_tagger(text)
                if w.feature.pos1 in ("名詞", "固有名詞")]
        return [normalize_ja(t) for t in toks if t not in ja_stop and len(t) > 1]
    else:  # default: English
        toks = nltk.word_tokenize(text)
        normalized_toks = [normalize_en(t) for t in toks
                          if re.fullmatch(r"\w+", t) and len(t) > 2]
        return [t for t in normalized_toks if t not in en_stop]

def extract_tags(title: str, content: str) -> list[str]:
    raw = f"{title}\n{content}"

    # Debug: Check input length
    if len(raw.strip()) < 10:
        print(f"Debug: Input too short ({len(raw)} chars)")
        return []

    print(f"Debug: Input length: {len(raw)} chars")
    print(f"Debug: Sample text: {raw[:200]}...")

    try:
        lang = detect(raw.replace("\n", " "))
    except:
        lang = "en"  # default to English if detection fails

    print(f"Debug: Detected language: {lang}")

    # Try KeyBERT directly on the original text first
    try:
        print("Debug: Testing KeyBERT with original text...")
        test_keywords = kw.extract_keywords(raw, top_n=5)
        print(f"Debug: Direct KeyBERT on original text: {test_keywords}")

        if test_keywords:
            # If direct approach works, use it
            result = [kw for kw, score in test_keywords if score >= 0.1][:5]
            print(f"Debug: Using direct approach, result: {result}")
            return result

    except Exception as e:
        print(f"Debug: Direct KeyBERT failed: {e}")

    # Fallback: try with processed candidates
    cands = candidate_tokens(raw, lang)
    print(f"Debug: Found {len(cands)} candidate tokens: {cands[:10]}...")

    if not cands:
        print("Debug: No candidates found")
        return []

    # Try with processed text
    try:
        text_for_keybert = " ".join(cands)
        print(f"Debug: Processed text length: {len(text_for_keybert)} chars")
        print(f"Debug: Sample processed text: {text_for_keybert[:200]}...")

        keywords = kw.extract_keywords(text_for_keybert, top_n=5)
        print(f"Debug: KeyBERT on processed text: {keywords}")

    except Exception as e:
        print(f"Debug: Processed KeyBERT failed: {e}")
        return []

    if not keywords:
        print("Debug: No keywords from processed text either")
        return []

    # Simple filtering
    result = [kw for kw, score in keywords if score >= 0.1][:5]
    print(f"Debug: Final result: {result}")
    return result
