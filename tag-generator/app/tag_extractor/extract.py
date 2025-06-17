module_name = "tag_extractor"

from sentence_transformers import SentenceTransformer
from keybert import KeyBERT
from langdetect import detect
from sentence_transformers import SentenceTransformer
from fugashi import Tagger
import nltk, unicodedata, re
import os


# モデル読み込みはスクリプト起動時に１回だけ
embedder = SentenceTransformer("paraphrase-multilingual-MiniLM-L12-v2")
kw = KeyBERT(embedder)
ja_tagger = Tagger("-Owakati")            # UniDicを事前インストール

# Get the directory of this file and construct paths to stopword files
current_dir = os.path.dirname(__file__)
ja_stopwords_path = os.path.join(current_dir, "stopwords_ja.txt")
en_stopwords_path = os.path.join(current_dir, "stopwords_en.txt")

# Load custom stopwords
ja_stop = set(open(ja_stopwords_path).read().split())
en_stop = set(open(en_stopwords_path).read().split())
# Combine with NLTK English stopwords for comprehensive coverage
en_stop.update(set(nltk.corpus.stopwords.words("english")))

def normalize_ja(t): return unicodedata.normalize("NFKC", t)
def normalize_en(t): return t.lower()

def candidate_tokens(text: str, lang: str) -> list[str]:
    "Tokenize + stop-word removal (lang‐specific)"
    if lang == "ja":
        toks = [w.surface for w in ja_tagger(text)
                if w.feature.pos1 in ("名詞", "固有名詞")]
        return [normalize_ja(t) for t in toks if t not in ja_stop and len(t) > 2]
    else:  # default: English
        toks = nltk.word_tokenize(text)
        return [normalize_en(t) for t in toks
                if t not in en_stop and re.fullmatch(r"\w+", t) and len(t) > 2]

def extract_tags(title: str, content: str) -> list[str]:
    raw = f"{title}\n{content}"
    try:
        lang = detect(raw.replace("\n", " "))
    except:
        lang = "en"  # default to English if detection fails

    cands = candidate_tokens(raw, lang)
    if not cands:
        return []

    # Use higher thresholds for better quality
    tags = [kw for kw, score in kw.extract_keywords(
        " ".join(cands),
        candidates=cands,
        use_mmr=True,  # Enable MMR for diversity
        diversity=0.7,  # Add diversity parameter
        threshold=0.4,  # Raise threshold from 0.25 to 0.4
        top_n=20       # Get more candidates initially
    ) if score >= 0.5]  # Higher final threshold from 0.35 to 0.5

    # Enhanced post-processing: dedupe, length filter, and quality checks
    seen, clean = set(), []
    for t in tags:
        # More stringent filtering
        if (t not in seen and
            2 < len(t) < 25 and  # Tighter length constraints
            not t.isdigit() and  # Exclude pure numbers
            not re.match(r'^[0-9]+$', t)):  # Exclude numeric strings
            clean.append(t)
            seen.add(t)

    return clean[:8]  # Reduce to max 8 high-quality tags
