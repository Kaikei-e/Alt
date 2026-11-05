"""Keep shared Kratos template inputs wired for the migration entrypoint."""

from pathlib import Path
import re
import unittest


class TestKratosMigrationEnvironment(unittest.TestCase):
    def test_migration_gets_the_server_template_urls(self):
        compose = (Path(__file__).resolve().parents[2] / "compose/auth.yaml").read_text()

        def environment(service):
            section = re.search(
                rf"^  {service}:\n(.*?)(?=^  [\w-]+:|\Z)",
                compose,
                re.MULTILINE | re.DOTALL,
            ).group(1)
            return dict(re.findall(r"^      - (KRATOS_[A-Z0-9_]+)=(.*)$", section, re.MULTILINE))

        server = environment("kratos")
        migration = environment("kratos-migrate")
        urls = {key: value for key, value in server.items() if "URL" in key or "CORS" in key}
        self.assertIn("KRATOS_ALLOWED_RETURN_URL_1", urls)
        for key, value in urls.items():
            with self.subTest(variable=key):
                self.assertEqual(migration.get(key), value)


if __name__ == "__main__":
    unittest.main()
