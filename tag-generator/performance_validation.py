#!/usr/bin/env python3
"""
Performance validation script for tag-generator Docker optimizations

This script tests and compares the performance of different Dockerfile optimizations
including build time, image size, startup time, and runtime performance.
"""

import os
import sys
import time
import json
import subprocess
import psutil
import logging
from pathlib import Path
from typing import Dict, List, Tuple, Any
from dataclasses import dataclass, asdict
from datetime import datetime
import asyncio

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

@dataclass
class PerformanceMetrics:
    """Performance metrics for Docker image optimization"""
    dockerfile_name: str
    build_time_seconds: float
    image_size_mb: float
    startup_time_seconds: float
    memory_usage_mb: float
    tag_extraction_time_ms: float
    build_success: bool
    runtime_success: bool
    error_message: str = ""

class DockerPerformanceTester:
    """Test Docker image optimization performance"""
    
    def __init__(self):
        self.dockerfiles = [
            "Dockerfile.tag-generator",  # Original
            "Dockerfile.tag-generator-optimized",  # Basic multi-stage
            "Dockerfile.tag-generator-ultra-optimized",  # Ultra optimized
            "Dockerfile.tag-generator-mecab-optimized",  # MecAB optimized
            "Dockerfile.tag-generator-external-models",  # External models
            "Dockerfile.tag-generator-alpine",  # Alpine version
        ]
        self.results: List[PerformanceMetrics] = []
        
    def run_command(self, cmd: List[str], timeout: int = 300) -> Tuple[int, str, str]:
        """Run a command and return exit code, stdout, stderr"""
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=timeout
            )
            return result.returncode, result.stdout, result.stderr
        except subprocess.TimeoutExpired:
            return -1, "", f"Command timed out after {timeout} seconds"
        except Exception as e:
            return -1, "", str(e)
    
    def build_image(self, dockerfile: str) -> Tuple[bool, float, str]:
        """Build Docker image and measure build time"""
        if not Path(dockerfile).exists():
            logger.warning(f"Dockerfile {dockerfile} does not exist, skipping")
            return False, 0.0, f"Dockerfile {dockerfile} not found"
        
        image_name = f"tag-generator:{dockerfile.split('.')[-1]}"
        
        logger.info(f"Building image {image_name} from {dockerfile}")
        
        build_cmd = [
            "docker", "build",
            "-f", dockerfile,
            "-t", image_name,
            "."
        ]
        
        start_time = time.time()
        exit_code, stdout, stderr = self.run_command(build_cmd, timeout=600)
        build_time = time.time() - start_time
        
        if exit_code == 0:
            logger.info(f"✅ Build successful: {image_name} in {build_time:.2f}s")
            return True, build_time, ""
        else:
            logger.error(f"❌ Build failed: {image_name} - {stderr}")
            return False, build_time, stderr
    
    def get_image_size(self, dockerfile: str) -> float:
        """Get image size in MB"""
        image_name = f"tag-generator:{dockerfile.split('.')[-1]}"
        
        cmd = ["docker", "images", "--format", "{{.Size}}", image_name]
        exit_code, stdout, stderr = self.run_command(cmd)
        
        if exit_code == 0 and stdout.strip():
            size_str = stdout.strip()
            
            # Parse size string (e.g., "1.2GB", "500MB")
            if "GB" in size_str:
                size_mb = float(size_str.replace("GB", "")) * 1024
            elif "MB" in size_str:
                size_mb = float(size_str.replace("MB", ""))
            else:
                size_mb = 0.0
            
            return size_mb
        else:
            logger.error(f"Failed to get image size for {image_name}: {stderr}")
            return 0.0
    
    def test_startup_time(self, dockerfile: str) -> Tuple[bool, float, float]:
        """Test container startup time and memory usage"""
        image_name = f"tag-generator:{dockerfile.split('.')[-1]}"
        
        # Test startup time with a simple command
        cmd = [
            "docker", "run", "--rm",
            image_name,
            "python", "-c", "import sys; print('Container started successfully'); sys.exit(0)"
        ]
        
        start_time = time.time()
        exit_code, stdout, stderr = self.run_command(cmd, timeout=60)
        startup_time = time.time() - start_time
        
        if exit_code == 0:
            logger.info(f"✅ Startup test passed: {image_name} in {startup_time:.2f}s")
            
            # Test memory usage
            memory_mb = self.get_container_memory_usage(image_name)
            return True, startup_time, memory_mb
        else:
            logger.error(f"❌ Startup test failed: {image_name} - {stderr}")
            return False, startup_time, 0.0
    
    def get_container_memory_usage(self, image_name: str) -> float:
        """Get container memory usage in MB"""
        try:
            # Run container in background
            run_cmd = [
                "docker", "run", "-d", "--name", "test-container",
                image_name,
                "python", "-c", "import time; time.sleep(30)"
            ]
            
            exit_code, stdout, stderr = self.run_command(run_cmd)
            
            if exit_code == 0:
                container_id = stdout.strip()
                
                # Get memory stats
                stats_cmd = ["docker", "stats", "--no-stream", "--format", "{{.MemUsage}}", container_id]
                exit_code, stdout, stderr = self.run_command(stats_cmd)
                
                # Cleanup
                self.run_command(["docker", "stop", container_id])
                self.run_command(["docker", "rm", container_id])
                
                if exit_code == 0 and stdout.strip():
                    # Parse memory usage (e.g., "123.4MiB / 1.5GiB")
                    memory_str = stdout.strip().split('/')[0].strip()
                    if "MiB" in memory_str:
                        return float(memory_str.replace("MiB", ""))
                    elif "GiB" in memory_str:
                        return float(memory_str.replace("GiB", "")) * 1024
            
            return 0.0
            
        except Exception as e:
            logger.error(f"Error getting memory usage: {e}")
            # Cleanup on error
            self.run_command(["docker", "stop", "test-container"])
            self.run_command(["docker", "rm", "test-container"])
            return 0.0
    
    def test_tag_extraction_performance(self, dockerfile: str) -> Tuple[bool, float]:
        """Test tag extraction performance"""
        image_name = f"tag-generator:{dockerfile.split('.')[-1]}"
        
        # Test tag extraction with sample data
        test_script = """
import time
import asyncio
import sys
import os

async def test_extraction():
    try:
        # Add current directory to path
        sys.path.insert(0, '/home/app')
        
        # Test lazy loading version if available
        try:
            from tag_extractor.extract_with_lazy_loading import extract_tags_async
            
            start_time = time.time()
            tags = await extract_tags_async(
                "Machine Learning and AI Development",
                "This article discusses machine learning algorithms, neural networks, and artificial intelligence applications in modern software development."
            )
            extraction_time = time.time() - start_time
            
            print(f"SUCCESS:{extraction_time*1000:.2f}:{len(tags)}:{tags}")
            
        except ImportError:
            # Fallback to regular extraction
            from tag_extractor.extract import extract_tags
            
            start_time = time.time()
            tags = extract_tags(
                "Machine Learning and AI Development",
                "This article discusses machine learning algorithms, neural networks, and artificial intelligence applications in modern software development."
            )
            extraction_time = time.time() - start_time
            
            print(f"SUCCESS:{extraction_time*1000:.2f}:{len(tags)}:{tags}")
            
    except Exception as e:
        print(f"ERROR:{e}")

if __name__ == "__main__":
    asyncio.run(test_extraction())
"""
        
        # Create temporary test file
        with open("temp_test.py", "w") as f:
            f.write(test_script)
        
        try:
            # Run test in container
            cmd = [
                "docker", "run", "--rm",
                "-v", f"{os.getcwd()}/temp_test.py:/home/app/temp_test.py",
                image_name,
                "python", "/home/app/temp_test.py"
            ]
            
            exit_code, stdout, stderr = self.run_command(cmd, timeout=120)
            
            if exit_code == 0 and stdout.strip():
                result = stdout.strip()
                if result.startswith("SUCCESS:"):
                    parts = result.split(":")
                    extraction_time_ms = float(parts[1])
                    tag_count = int(parts[2])
                    
                    logger.info(f"✅ Tag extraction test passed: {image_name} - {extraction_time_ms:.2f}ms, {tag_count} tags")
                    return True, extraction_time_ms
                else:
                    logger.error(f"❌ Tag extraction test failed: {image_name} - {result}")
                    return False, 0.0
            else:
                logger.error(f"❌ Tag extraction test failed: {image_name} - {stderr}")
                return False, 0.0
                
        finally:
            # Cleanup
            if os.path.exists("temp_test.py"):
                os.remove("temp_test.py")
    
    def test_dockerfile(self, dockerfile: str) -> PerformanceMetrics:
        """Test a single Dockerfile and return metrics"""
        logger.info(f"Testing Dockerfile: {dockerfile}")
        
        # Build image
        build_success, build_time, build_error = self.build_image(dockerfile)
        
        if not build_success:
            return PerformanceMetrics(
                dockerfile_name=dockerfile,
                build_time_seconds=build_time,
                image_size_mb=0.0,
                startup_time_seconds=0.0,
                memory_usage_mb=0.0,
                tag_extraction_time_ms=0.0,
                build_success=False,
                runtime_success=False,
                error_message=build_error
            )
        
        # Get image size
        image_size = self.get_image_size(dockerfile)
        
        # Test startup
        startup_success, startup_time, memory_usage = self.test_startup_time(dockerfile)
        
        if not startup_success:
            return PerformanceMetrics(
                dockerfile_name=dockerfile,
                build_time_seconds=build_time,
                image_size_mb=image_size,
                startup_time_seconds=startup_time,
                memory_usage_mb=0.0,
                tag_extraction_time_ms=0.0,
                build_success=True,
                runtime_success=False,
                error_message="Container startup failed"
            )
        
        # Test tag extraction
        extraction_success, extraction_time = self.test_tag_extraction_performance(dockerfile)
        
        return PerformanceMetrics(
            dockerfile_name=dockerfile,
            build_time_seconds=build_time,
            image_size_mb=image_size,
            startup_time_seconds=startup_time,
            memory_usage_mb=memory_usage,
            tag_extraction_time_ms=extraction_time,
            build_success=True,
            runtime_success=extraction_success,
            error_message="" if extraction_success else "Tag extraction failed"
        )
    
    def run_all_tests(self) -> List[PerformanceMetrics]:
        """Run all performance tests"""
        logger.info("Starting performance validation tests...")
        
        results = []
        
        for dockerfile in self.dockerfiles:
            try:
                metrics = self.test_dockerfile(dockerfile)
                results.append(metrics)
                
                # Log summary
                logger.info(f"Completed {dockerfile}:")
                logger.info(f"  Build: {'✅' if metrics.build_success else '❌'} {metrics.build_time_seconds:.2f}s")
                logger.info(f"  Size: {metrics.image_size_mb:.1f}MB")
                logger.info(f"  Startup: {'✅' if metrics.runtime_success else '❌'} {metrics.startup_time_seconds:.2f}s")
                logger.info(f"  Memory: {metrics.memory_usage_mb:.1f}MB")
                logger.info(f"  Extraction: {metrics.tag_extraction_time_ms:.2f}ms")
                
            except Exception as e:
                logger.error(f"Error testing {dockerfile}: {e}")
                results.append(PerformanceMetrics(
                    dockerfile_name=dockerfile,
                    build_time_seconds=0.0,
                    image_size_mb=0.0,
                    startup_time_seconds=0.0,
                    memory_usage_mb=0.0,
                    tag_extraction_time_ms=0.0,
                    build_success=False,
                    runtime_success=False,
                    error_message=str(e)
                ))
        
        return results
    
    def generate_report(self, results: List[PerformanceMetrics]) -> str:
        """Generate performance report"""
        report = []
        report.append("="*80)
        report.append("DOCKER OPTIMIZATION PERFORMANCE REPORT")
        report.append("="*80)
        report.append(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        report.append("")
        
        # Find baseline (original Dockerfile)
        baseline = None
        for result in results:
            if result.dockerfile_name == "Dockerfile.tag-generator":
                baseline = result
                break
        
        # Summary table
        report.append("SUMMARY TABLE:")
        report.append("-" * 80)
        report.append(f"{'Dockerfile':<35} {'Build':<8} {'Size(MB)':<10} {'Startup':<8} {'Memory':<8} {'Extract':<8}")
        report.append("-" * 80)
        
        for result in results:
            build_status = "✅" if result.build_success else "❌"
            runtime_status = "✅" if result.runtime_success else "❌"
            
            dockerfile_short = result.dockerfile_name.replace("Dockerfile.tag-generator", "").replace("-", "") or "original"
            
            report.append(f"{dockerfile_short:<35} {build_status:<8} {result.image_size_mb:>8.1f} {result.startup_time_seconds:>6.2f}s {result.memory_usage_mb:>6.1f}MB {result.tag_extraction_time_ms:>6.0f}ms")
        
        # Detailed analysis
        report.append("\n\nDETAILED ANALYSIS:")
        report.append("-" * 80)
        
        for result in results:
            dockerfile_short = result.dockerfile_name.replace("Dockerfile.tag-generator", "").replace("-", "") or "original"
            
            report.append(f"\n{dockerfile_short.upper()}:")
            report.append(f"  Build Success: {'✅' if result.build_success else '❌'}")
            report.append(f"  Runtime Success: {'✅' if result.runtime_success else '❌'}")
            report.append(f"  Build Time: {result.build_time_seconds:.2f}s")
            report.append(f"  Image Size: {result.image_size_mb:.1f}MB")
            report.append(f"  Startup Time: {result.startup_time_seconds:.2f}s")
            report.append(f"  Memory Usage: {result.memory_usage_mb:.1f}MB")
            report.append(f"  Tag Extraction: {result.tag_extraction_time_ms:.2f}ms")
            
            if baseline and result != baseline and result.build_success:
                # Calculate improvements
                if baseline.image_size_mb > 0:
                    size_improvement = (baseline.image_size_mb - result.image_size_mb) / baseline.image_size_mb * 100
                    report.append(f"  Size Improvement: {size_improvement:+.1f}%")
                
                if baseline.build_time_seconds > 0:
                    build_improvement = (baseline.build_time_seconds - result.build_time_seconds) / baseline.build_time_seconds * 100
                    report.append(f"  Build Time Improvement: {build_improvement:+.1f}%")
                
                if baseline.startup_time_seconds > 0:
                    startup_improvement = (baseline.startup_time_seconds - result.startup_time_seconds) / baseline.startup_time_seconds * 100
                    report.append(f"  Startup Time Improvement: {startup_improvement:+.1f}%")
            
            if result.error_message:
                report.append(f"  Error: {result.error_message}")
        
        # Recommendations
        report.append("\n\nRECOMMENDATIONS:")
        report.append("-" * 80)
        
        successful_results = [r for r in results if r.build_success and r.runtime_success]
        
        if successful_results:
            # Best size optimization
            best_size = min(successful_results, key=lambda x: x.image_size_mb)
            report.append(f"Best Size Optimization: {best_size.dockerfile_name} ({best_size.image_size_mb:.1f}MB)")
            
            # Best build time
            best_build = min(successful_results, key=lambda x: x.build_time_seconds)
            report.append(f"Fastest Build: {best_build.dockerfile_name} ({best_build.build_time_seconds:.2f}s)")
            
            # Best startup time
            best_startup = min(successful_results, key=lambda x: x.startup_time_seconds)
            report.append(f"Fastest Startup: {best_startup.dockerfile_name} ({best_startup.startup_time_seconds:.2f}s)")
            
            # Best overall (balanced score)
            for result in successful_results:
                if baseline and baseline.image_size_mb > 0:
                    size_score = (baseline.image_size_mb - result.image_size_mb) / baseline.image_size_mb
                    build_score = (baseline.build_time_seconds - result.build_time_seconds) / baseline.build_time_seconds if baseline.build_time_seconds > 0 else 0
                    startup_score = (baseline.startup_time_seconds - result.startup_time_seconds) / baseline.startup_time_seconds if baseline.startup_time_seconds > 0 else 0
                    
                    overall_score = (size_score + build_score + startup_score) / 3
                    result.overall_score = overall_score
            
            if hasattr(successful_results[0], 'overall_score'):
                best_overall = max(successful_results, key=lambda x: getattr(x, 'overall_score', 0))
                report.append(f"Best Overall: {best_overall.dockerfile_name} (score: {getattr(best_overall, 'overall_score', 0):.3f})")
        
        return "\n".join(report)
    
    def save_results(self, results: List[PerformanceMetrics], filename: str = "performance_results.json"):
        """Save results to JSON file"""
        with open(filename, 'w') as f:
            json.dump([asdict(result) for result in results], f, indent=2)
        logger.info(f"Results saved to {filename}")


def main():
    """Main function"""
    tester = DockerPerformanceTester()
    
    try:
        # Run all tests
        results = tester.run_all_tests()
        
        # Generate report
        report = tester.generate_report(results)
        
        # Print report
        print(report)
        
        # Save results
        tester.save_results(results)
        
        # Save report
        with open("performance_report.txt", "w") as f:
            f.write(report)
        
        logger.info("Performance validation completed successfully!")
        
        # Check if any tests failed
        failed_tests = [r for r in results if not r.build_success or not r.runtime_success]
        if failed_tests:
            logger.warning(f"{len(failed_tests)} tests failed")
            sys.exit(1)
        else:
            logger.info("All tests passed!")
            sys.exit(0)
            
    except Exception as e:
        logger.error(f"Performance validation failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()