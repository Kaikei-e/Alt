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

# Get the directory of this file and construct the path to stopwords_ja.txt
current_dir = os.path.dirname(__file__)
stopwords_path = os.path.join(current_dir, "stopwords_ja.txt")
ja_stop = set(open(stopwords_path).read().split())
en_stop = set(nltk.corpus.stopwords.words("english"))

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
        return [normalize_en(t) for t in toks
                if t not in en_stop and re.fullmatch(r"\w+", t)]

def extract_tags(title: str, content: str) -> list[str]:
    raw = f"{title}\n{content}"
    try:
        lang = detect(raw.replace("\n", " "))
    except:
        lang = "en"  # default to English if detection fails

    cands = candidate_tokens(raw, lang)
    if not cands:
        return []
    tags = [kw for kw, score in kw.extract_keywords(
        " ".join(cands),
        candidates=cands,
        use_mmr=False,
        threshold=0.25,
        top_n=15
    ) if score >= 0.35]
    # Optional post-processing: dedupe & length filter
    seen, clean = set(), []
    for t in tags:
        if t not in seen and 1 < len(t) < 30:
            clean.append(t)
            seen.add(t)
    return clean[:10]  # feed max-10 tags to Meilisearch
