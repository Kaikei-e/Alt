#!/usr/bin/env python3
"""
Alpine Linux compatibility testing script for tag-generator service

This script tests whether all required packages can be imported and function
correctly on Alpine Linux with musl libc.
"""

import sys
import os
import importlib
import logging
import platform
import time
from pathlib import Path
from typing import Dict, List, Tuple, Any

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

class AlpineCompatibilityTester:
    """Test Alpine Linux compatibility for tag-generator dependencies"""
    
    def __init__(self):
        self.results = {}
        self.errors = []
        self.warnings = []
        
    def print_system_info(self):
        """Print system information"""
        logger.info("System Information:")
        logger.info(f"  Platform: {platform.platform()}")
        logger.info(f"  Python: {sys.version}")
        logger.info(f"  Architecture: {platform.machine()}")
        logger.info(f"  Libc: {platform.libc_ver()}")
        
        # Check if we're on Alpine
        if os.path.exists('/etc/alpine-release'):
            with open('/etc/alpine-release') as f:
                alpine_version = f.read().strip()
            logger.info(f"  Alpine version: {alpine_version}")
        else:
            logger.warning("  Not running on Alpine Linux")
    
    def test_package_imports(self) -> Dict[str, str]:
        """Test if all required packages can be imported"""
        logger.info("Testing package imports...")
        
        # Core dependencies
        packages = [
            # Core Python packages
            'structlog',
            'pydantic',
            'bleach',
            'langdetect',
            'psutil',
            
            # Database
            'psycopg2',
            
            # Text processing
            'fugashi',
            'nltk',
            
            # ML/AI packages (most likely to have issues)
            'sentence_transformers',
            'transformers',
            'keybert',
            
            # Development (if available)
            'pytest',
            'ruff',
        ]
        
        results = {}
        
        for package in packages:
            try:
                start_time = time.time()
                importlib.import_module(package)
                import_time = time.time() - start_time
                results[package] = f"✅ SUCCESS ({import_time:.2f}s)"
                logger.info(f"✅ {package} imported successfully in {import_time:.2f}s")
            except ImportError as e:
                results[package] = f"❌ IMPORT_ERROR: {e}"
                logger.error(f"❌ {package} import failed: {e}")
                self.errors.append(f"Import failed: {package} - {e}")
            except Exception as e:
                results[package] = f"⚠️ ERROR: {e}"
                logger.warning(f"⚠️ {package} import error: {e}")
                self.warnings.append(f"Import error: {package} - {e}")
        
        return results
    
    def test_fugashi_functionality(self) -> Tuple[str, str]:
        """Test Fugashi Japanese text processing"""
        logger.info("Testing Fugashi functionality...")
        
        try:
            import fugashi
            
            # Test basic tagger creation
            tagger = fugashi.Tagger()
            
            # Test Japanese text parsing
            test_text = "これはテストです。"
            result = tagger.parse(test_text)
            
            if result:
                logger.info(f"✅ Fugashi test passed: {result.strip()}")
                return "✅ SUCCESS", "Basic parsing works"
            else:
                logger.warning("⚠️ Fugashi returned empty result")
                return "⚠️ WARNING", "Empty result"
                
        except Exception as e:
            logger.error(f"❌ Fugashi test failed: {e}")
            return "❌ FAILED", str(e)
    
    def test_sentence_transformers_functionality(self) -> Tuple[str, str]:
        """Test SentenceTransformer functionality"""
        logger.info("Testing SentenceTransformer functionality...")
        
        try:
            from sentence_transformers import SentenceTransformer
            
            # Use a small model for testing
            model_name = 'all-MiniLM-L6-v2'
            logger.info(f"Loading model: {model_name}")
            
            model = SentenceTransformer(model_name, device='cpu')
            
            # Test encoding
            test_text = "This is a test sentence."
            embedding = model.encode(test_text)
            
            if embedding is not None and len(embedding) > 0:
                logger.info(f"✅ SentenceTransformer test passed: embedding shape {embedding.shape}")
                return "✅ SUCCESS", f"Embedding shape: {embedding.shape}"
            else:
                logger.warning("⚠️ SentenceTransformer returned empty embedding")
                return "⚠️ WARNING", "Empty embedding"
                
        except Exception as e:
            logger.error(f"❌ SentenceTransformer test failed: {e}")
            return "❌ FAILED", str(e)
    
    def test_keybert_functionality(self) -> Tuple[str, str]:
        """Test KeyBERT functionality"""
        logger.info("Testing KeyBERT functionality...")
        
        try:
            from keybert import KeyBERT
            
            # Create KeyBERT instance
            kw_model = KeyBERT()
            
            # Test keyword extraction
            test_text = """
                Machine learning is a subset of artificial intelligence that focuses on
                algorithms that can learn from data. Deep learning is a subset of machine
                learning that uses neural networks with multiple layers.
            """
            
            keywords = kw_model.extract_keywords(test_text, keyphrase_ngram_range=(1, 2), top_n=5)
            
            if keywords and len(keywords) > 0:
                logger.info(f"✅ KeyBERT test passed: {keywords}")
                return "✅ SUCCESS", f"Keywords: {keywords}"
            else:
                logger.warning("⚠️ KeyBERT returned no keywords")
                return "⚠️ WARNING", "No keywords extracted"
                
        except Exception as e:
            logger.error(f"❌ KeyBERT test failed: {e}")
            return "❌ FAILED", str(e)
    
    def test_nltk_functionality(self) -> Tuple[str, str]:
        """Test NLTK functionality"""
        logger.info("Testing NLTK functionality...")
        
        try:
            import nltk
            from nltk.tokenize import word_tokenize
            
            # Test tokenization
            test_text = "This is a test sentence for NLTK."
            tokens = word_tokenize(test_text)
            
            if tokens and len(tokens) > 0:
                logger.info(f"✅ NLTK test passed: {tokens}")
                return "✅ SUCCESS", f"Tokens: {tokens}"
            else:
                logger.warning("⚠️ NLTK returned no tokens")
                return "⚠️ WARNING", "No tokens"
                
        except LookupError as e:
            logger.warning(f"⚠️ NLTK data not available: {e}")
            return "⚠️ DATA_MISSING", str(e)
        except Exception as e:
            logger.error(f"❌ NLTK test failed: {e}")
            return "❌ FAILED", str(e)
    
    def test_database_connectivity(self) -> Tuple[str, str]:
        """Test database connectivity (psycopg2)"""
        logger.info("Testing database connectivity...")
        
        try:
            import psycopg2
            
            # Test basic psycopg2 functionality (no actual connection)
            version = psycopg2.__version__
            logger.info(f"✅ psycopg2 version: {version}")
            return "✅ SUCCESS", f"Version: {version}"
            
        except Exception as e:
            logger.error(f"❌ psycopg2 test failed: {e}")
            return "❌ FAILED", str(e)
    
    def test_performance_critical_packages(self) -> Dict[str, Tuple[str, str]]:
        """Test performance-critical packages"""
        logger.info("Testing performance-critical packages...")
        
        tests = {
            'fugashi': self.test_fugashi_functionality,
            'sentence_transformers': self.test_sentence_transformers_functionality,
            'keybert': self.test_keybert_functionality,
            'nltk': self.test_nltk_functionality,
            'psycopg2': self.test_database_connectivity,
        }
        
        results = {}
        
        for test_name, test_func in tests.items():
            try:
                status, message = test_func()
                results[test_name] = (status, message)
            except Exception as e:
                results[test_name] = ("❌ EXCEPTION", str(e))
                self.errors.append(f"Test exception: {test_name} - {e}")
        
        return results
    
    def test_memory_usage(self) -> Dict[str, Any]:
        """Test memory usage of key components"""
        logger.info("Testing memory usage...")
        
        try:
            import psutil
            import os
            
            process = psutil.Process(os.getpid())
            
            # Memory before loading heavy packages
            memory_before = process.memory_info().rss / 1024 / 1024  # MB
            
            # Load heavy packages
            import sentence_transformers
            import transformers
            
            # Memory after loading
            memory_after = process.memory_info().rss / 1024 / 1024  # MB
            
            return {
                'memory_before_mb': memory_before,
                'memory_after_mb': memory_after,
                'memory_increase_mb': memory_after - memory_before,
                'status': '✅ SUCCESS'
            }
            
        except Exception as e:
            logger.error(f"❌ Memory test failed: {e}")
            return {
                'status': '❌ FAILED',
                'error': str(e)
            }
    
    def run_all_tests(self) -> Dict[str, Any]:
        """Run all compatibility tests"""
        logger.info("Starting Alpine compatibility tests...")
        
        # Print system info
        self.print_system_info()
        
        # Test package imports
        import_results = self.test_package_imports()
        
        # Test functionality
        functionality_results = self.test_performance_critical_packages()
        
        # Test memory usage
        memory_results = self.test_memory_usage()
        
        # Compile results
        results = {
            'system_info': {
                'platform': platform.platform(),
                'python_version': sys.version,
                'is_alpine': os.path.exists('/etc/alpine-release'),
                'libc': platform.libc_ver()
            },
            'import_tests': import_results,
            'functionality_tests': functionality_results,
            'memory_tests': memory_results,
            'errors': self.errors,
            'warnings': self.warnings
        }
        
        return results
    
    def print_summary(self, results: Dict[str, Any]):
        """Print test summary"""
        print("\n" + "="*60)
        print("ALPINE COMPATIBILITY TEST SUMMARY")
        print("="*60)
        
        # Import tests
        print("\n1. PACKAGE IMPORT TESTS:")
        for package, result in results['import_tests'].items():
            print(f"   {package:20} {result}")
        
        # Functionality tests
        print("\n2. FUNCTIONALITY TESTS:")
        for test_name, (status, message) in results['functionality_tests'].items():
            print(f"   {test_name:20} {status}")
            if message:
                print(f"                        {message}")
        
        # Memory tests
        print("\n3. MEMORY USAGE TEST:")
        memory = results['memory_tests']
        if memory['status'] == '✅ SUCCESS':
            print(f"   Memory before:       {memory['memory_before_mb']:.1f} MB")
            print(f"   Memory after:        {memory['memory_after_mb']:.1f} MB")
            print(f"   Memory increase:     {memory['memory_increase_mb']:.1f} MB")
        else:
            print(f"   Memory test:         {memory['status']}")
        
        # Errors and warnings
        if self.errors:
            print(f"\n4. ERRORS ({len(self.errors)}):")
            for error in self.errors:
                print(f"   ❌ {error}")
        
        if self.warnings:
            print(f"\n5. WARNINGS ({len(self.warnings)}):")
            for warning in self.warnings:
                print(f"   ⚠️ {warning}")
        
        # Overall status
        failed_imports = sum(1 for result in results['import_tests'].values() if '❌' in result)
        failed_functions = sum(1 for status, _ in results['functionality_tests'].values() if '❌' in status)
        
        print(f"\n6. OVERALL STATUS:")
        print(f"   Failed imports:      {failed_imports}")
        print(f"   Failed functions:    {failed_functions}")
        print(f"   Errors:              {len(self.errors)}")
        print(f"   Warnings:            {len(self.warnings)}")
        
        if failed_imports == 0 and failed_functions == 0 and len(self.errors) == 0:
            print(f"   Result:              ✅ FULLY COMPATIBLE")
            return True
        elif failed_imports == 0 and failed_functions <= 2:
            print(f"   Result:              ⚠️ MOSTLY COMPATIBLE")
            return True
        else:
            print(f"   Result:              ❌ COMPATIBILITY ISSUES")
            return False


def main():
    """Main function"""
    tester = AlpineCompatibilityTester()
    
    try:
        results = tester.run_all_tests()
        success = tester.print_summary(results)
        
        if success:
            logger.info("Alpine compatibility tests completed successfully!")
            sys.exit(0)
        else:
            logger.error("Alpine compatibility issues found!")
            sys.exit(1)
            
    except Exception as e:
        logger.error(f"Test runner failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()