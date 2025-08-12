from setuptools import setup, find_packages

setup(
    name="alt-auth",
    version="1.0.0",
    description="Alt Authentication Library for Python Services",
    packages=find_packages(),
    install_requires=[
        "aiohttp>=3.9.0",
        "PyJWT>=2.8.0",
        "fastapi>=0.100.0",
        "python-multipart>=0.0.6",
    ],
    python_requires=">=3.9",
)