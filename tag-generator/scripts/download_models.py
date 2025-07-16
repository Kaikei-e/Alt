#!/usr/bin/env python3
"""Model download script for external storage strategy"""

import os
import sys
from pathlib import Path
import logging
import asyncio
import time

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

class ModelDownloader:
    def __init__(self, models_dir: str = "/models"):
        self.models_dir = Path(models_dir)
        self.models_dir.mkdir(parents=True, exist_ok=True)
        
    def download_all_models(self):
        """Download all required models"""
        logger.info(f"Starting model download to {self.models_dir}")
        start_time = time.time()
        
        try:
            # Download models in parallel for better performance
            self._download_nltk_data()
            self._download_unidic()
            self._download_sentence_transformer()
            
            elapsed = time.time() - start_time
            logger.info(f"All models downloaded successfully in {elapsed:.2f} seconds")
            return True
            
        except Exception as e:
            logger.error(f"Model download failed: {e}")
            return False
    
    def _download_nltk_data(self):
        """Download NLTK data to specified directory"""
        nltk_dir = self.models_dir / 'nltk_data'
        nltk_dir.mkdir(exist_ok=True)
        
        logger.info("Downloading NLTK data...")
        
        import nltk
        
        # Download required NLTK data
        datasets = ['stopwords', 'punkt', 'punkt_tab']
        for dataset in datasets:
            try:
                nltk.download(dataset, download_dir=str(nltk_dir), quiet=True)
                logger.info(f"Downloaded NLTK {dataset}")
            except Exception as e:
                logger.warning(f"Failed to download NLTK {dataset}: {e}")
                
        # Verify downloads
        expected_files = [
            nltk_dir / 'corpora' / 'stopwords',
            nltk_dir / 'tokenizers' / 'punkt',
        ]
        
        for file_path in expected_files:
            if file_path.exists():
                logger.info(f"Verified: {file_path}")
            else:
                logger.warning(f"Missing: {file_path}")
    
    def _download_unidic(self):
        """Download UniDic for Japanese text processing"""
        logger.info("Downloading UniDic...")
        
        try:
            import unidic
            unidic.download()
            logger.info("UniDic downloaded successfully")
            
            # Verify UniDic installation
            if hasattr(unidic, 'DICDIR') and Path(unidic.DICDIR).exists():
                logger.info(f"UniDic verified at: {unidic.DICDIR}")
            else:
                logger.warning("UniDic verification failed")
                
        except ImportError:
            logger.error("UniDic package not found. Install with: pip install unidic")
        except Exception as e:
            logger.error(f"UniDic download failed: {e}")
            logger.info("Will try alternative approach during runtime")
    
    def _download_sentence_transformer(self):
        """Download SentenceTransformer model"""
        st_dir = self.models_dir / 'sentence_transformers'
        st_dir.mkdir(exist_ok=True)
        
        logger.info("Downloading SentenceTransformer model...")
        
        # Set cache directory
        os.environ['SENTENCE_TRANSFORMERS_HOME'] = str(st_dir)
        
        try:
            from sentence_transformers import SentenceTransformer
            
            model_name = 'paraphrase-multilingual-MiniLM-L12-v2'
            
            # Download and cache model
            model = SentenceTransformer(model_name, device='cpu')
            
            # Verify model is cached
            model_path = st_dir / model_name
            if model_path.exists():
                logger.info(f"SentenceTransformer model cached at: {model_path}")
            else:
                logger.warning("SentenceTransformer model cache verification failed")
                
        except ImportError:
            logger.error("SentenceTransformer package not found. Install with: pip install sentence-transformers")
        except Exception as e:
            logger.error(f"SentenceTransformer download failed: {e}")
    
    def verify_models(self):
        """Verify all models are properly downloaded"""
        logger.info("Verifying model downloads...")
        
        checks = [
            ("NLTK stopwords", self.models_dir / 'nltk_data' / 'corpora' / 'stopwords'),
            ("NLTK punkt", self.models_dir / 'nltk_data' / 'tokenizers' / 'punkt'),
            ("SentenceTransformer", self.models_dir / 'sentence_transformers' / 'paraphrase-multilingual-MiniLM-L12-v2'),
        ]
        
        all_good = True
        for name, path in checks:
            if path.exists():
                logger.info(f"✓ {name}: {path}")
            else:
                logger.warning(f"✗ {name}: {path}")
                all_good = False
        
        # Check UniDic separately
        try:
            import unidic
            if hasattr(unidic, 'DICDIR') and Path(unidic.DICDIR).exists():
                logger.info(f"✓ UniDic: {unidic.DICDIR}")
            else:
                logger.warning("✗ UniDic: Not found")
                all_good = False
        except Exception:
            logger.warning("✗ UniDic: Cannot verify")
            all_good = False
        
        return all_good

def main():
    """Main function for model download"""
    models_dir = os.environ.get('MODELS_DIR', '/models')
    
    logger.info(f"Model download script starting with MODELS_DIR={models_dir}")
    
    downloader = ModelDownloader(models_dir)
    
    # Download all models
    success = downloader.download_all_models()
    
    if success:
        # Verify downloads
        verified = downloader.verify_models()
        if verified:
            logger.info("All models downloaded and verified successfully!")
            sys.exit(0)
        else:
            logger.warning("Some models failed verification")
            sys.exit(1)
    else:
        logger.error("Model download failed")
        sys.exit(1)

if __name__ == "__main__":
    main()