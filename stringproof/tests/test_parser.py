from __future__ import annotations

from pathlib import Path
import tempfile
import unittest

from stringproof.audit import parse_catalog


class ParserTests(unittest.TestCase):
    def test_supported_encodings(self) -> None:
        variants = {
            "utf-8": '"title" = "Résumé";\n'.encode("utf-8"),
            "utf-8-sig": '"title" = "Résumé";\n'.encode("utf-8-sig"),
            "utf-16": '"title" = "Résumé";\n'.encode("utf-16"),
            "utf-16-le": '"title" = "Résumé";\n'.encode("utf-16-le"),
            "utf-16-be": '"title" = "Résumé";\n'.encode("utf-16-be"),
        }
        for name, data in variants.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as raw:
                root = Path(raw)
                path = root / "en.lproj" / "Localizable.strings"
                path.parent.mkdir()
                path.write_bytes(data)
                catalog, findings = parse_catalog(path, root)
                self.assertEqual(findings, [])
                self.assertEqual(catalog.entries[0].key, "title")
                self.assertEqual(catalog.entries[0].value, "Résumé")

    def test_malformed_syntax_and_duplicate_keys_are_findings(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            path = root / "en.lproj" / "Localizable.strings"
            path.parent.mkdir()
            path.write_text(
                '"good" = "one";\n'
                '"good" = "two";\n'
                '"broken" = "missing semicolon"\n'
                '"after" = "still parsed";\n',
                encoding="utf-8",
            )
            catalog, findings = parse_catalog(path, root)
            self.assertEqual(
                {finding.code for finding in findings},
                {"key.duplicate", "syntax.invalid"},
            )
            self.assertIn("after", catalog.first_by_key)

    def test_whitespace_before_semicolon_is_valid(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            path = root / "en.lproj" / "Localizable.strings"
            path.parent.mkdir()
            path.write_text('"key" = "value"\n  ;\n', encoding="utf-8")
            catalog, findings = parse_catalog(path, root)
            self.assertEqual(findings, [])
            self.assertEqual(catalog.entries[0].value, "value")

    def test_malformed_constructs_are_syntax_findings(self) -> None:
        cases = (
            '"key" = "missing semicolon"\n',
            '"key" = "unterminated value\n',
            "/* unterminated comment\n",
            '"key" = "\\U12G4";\n',
            '"key" = "\\UD800";\n',
        )
        for content in cases:
            with self.subTest(content=content), tempfile.TemporaryDirectory() as raw:
                root = Path(raw)
                path = root / "en.lproj" / "Localizable.strings"
                path.parent.mkdir()
                path.write_text(content, encoding="utf-8")
                _, findings = parse_catalog(path, root)
                self.assertIn(
                    "syntax.invalid",
                    {finding.code for finding in findings},
                )


if __name__ == "__main__":
    unittest.main()
