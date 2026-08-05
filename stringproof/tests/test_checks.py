from __future__ import annotations

import json
from pathlib import Path
import tempfile
import unittest

from stringproof.audit import check


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]


def check_pair(
    source_locale: str,
    source_value: str,
    target_locale: str,
    target_value: str,
) -> dict[str, object]:
    with tempfile.TemporaryDirectory() as raw:
        root = Path(raw)
        for locale, value in (
            (source_locale, source_value),
            (target_locale, target_value),
        ):
            folder = root / f"{locale}.lproj"
            folder.mkdir()
            (folder / "Localizable.strings").write_text(
                f'"key" = "{value}";\n',
                encoding="utf-8",
            )
        return check(root, source_locale)


class CheckTests(unittest.TestCase):
    def test_corrupted_fixture_covers_distinct_regressions(self) -> None:
        fixture = REPOSITORY_ROOT / "fixtures" / "corrupted"
        before = {
            path: path.read_bytes()
            for path in fixture.rglob("*.strings")
        }

        report = check(fixture, "en")

        expected = json.loads(
            (fixture / "expectations.json").read_text(encoding="utf-8")
        )
        actual: dict[str, list[str]] = {}
        for finding in report["findings"]:
            actual.setdefault(finding["key"], []).append(finding["code"])
        self.assertEqual(
            {key: sorted(codes) for key, codes in actual.items()},
            expected,
        )
        self.assertEqual(report["status"], "findings")
        self.assertEqual(
            before,
            {path: path.read_bytes() for path in fixture.rglob("*.strings")},
        )

    def test_structural_key_drift_and_all_placeholder_families(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "en.lproj" / "Localizable.strings"
            target = root / "de.lproj" / "Localizable.strings"
            source.parent.mkdir()
            target.parent.mkdir()
            source.write_text(
                '"all" = "%2$@ {count, plural, one {# item} other {# items}} \\\\(name)";\n'
                '"missing.key" = "Missing";\n',
                encoding="utf-8",
            )
            target.write_text(
                '"all" = "%d {total, plural, one {# Artikel} other {# Artikel}} \\\\(name)";\n'
                '"unexpected.key" = "Unerwartet";\n',
                encoding="utf-8",
            )

            findings = check(root, "en")["findings"]
            by_code = {}
            for finding in findings:
                by_code.setdefault(finding["code"], []).append(finding)

            self.assertIn("key.missing", by_code)
            self.assertIn("key.unexpected", by_code)
            self.assertEqual(
                by_code["placeholder.missing"][0]["message"],
                "missing placeholder(s): %2$@, {count}",
            )
            self.assertEqual(
                by_code["placeholder.added"][0]["message"],
                "added placeholder(s): %d, {total}",
            )

    def test_model_residue_exclusions(self) -> None:
        cases = (
            ("Example Product-backup.json", "fr", "example-product-backup.json"),
            ("Evening hymn", "fi", "Ilta-laulu"),
        )
        for source, locale, target in cases:
            with self.subTest(target=target):
                findings = check_pair("en", source, locale, target)["findings"]
                self.assertNotIn(
                    "model.residue",
                    {item["code"] for item in findings},
                )

    def test_unchanged_text_exclusions_remain_clean(self) -> None:
        report = check(REPOSITORY_ROOT / "fixtures" / "clean", "en")
        self.assertEqual(report["status"], "clean")
        self.assertEqual(report["findings"], [])

    def test_unicode_scripts_accept_shared_text_and_reject_real_contamination(self) -> None:
        cases = (
            ("pa", "ਐਪਾਂ ਚੁਣੋ। ਚੋਣਾਂ ਨਿੱਜੀ ਹਨ।", False),
            ("bn", "অ্যাপ বেছে নিন। নির্বাচন ব্যক্তিগত থাকে।", False),
            ("or", "ଆପ୍ ବାଛନ୍ତୁ। ଚୟନ ବ୍ୟକ୍ତିଗତ ରହେ।", False),
            ("mr", "ॲप्स आणि श्रेण्या निवडा.", False),
            ("ja", "カタカナ・テストー", False),
            ("pa-Arab", "ایپس تے زمرے چنو۔", False),
            ("pa-Guru", "ਐਪਾਂ ਅਤੇ ਸ਼੍ਰੇਣੀਆਂ ਚੁਣੋ।", False),
            ("hr", "Odaberite појединачно", True),
            ("ml", "അനുഭവം 만든", True),
            ("ja", "ようこそ welcome back", True),
            ("fr", "pаypal", True),
            ("el", "Οι ρυθμίσεις доступны", True),
            ("ru", "Настройки διαθέσιμες", True),
            ("he", "הגדרות متاحة", True),
            ("ko", "설정 ひらがな", True),
            ("ta", "அமைப்புகள் తెలుగు", True),
        )
        for locale, target, expected in cases:
            with self.subTest(locale=locale, target=target):
                findings = check_pair("en", "Choose apps and categories", locale, target)[
                    "findings"
                ]
                self.assertEqual(
                    any(item["code"] == "script.unexpected" for item in findings),
                    expected,
                )

    def test_unpaired_and_duplicate_catalogs_are_findings(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            catalogs = {
                "en.lproj/Localizable.strings": '"key" = "Source";\n',
                "en-US.lproj/Localizable.strings": '"key" = "Target";\n',
                "en_US.lproj/Localizable.strings": '"key" = "Duplicate";\n',
                "Feature/fr.lproj/Errors.strings": '"error" = "Erreur";\n',
            }
            for relative_path, content in catalogs.items():
                path = root / relative_path
                path.parent.mkdir(parents=True)
                path.write_text(content, encoding="utf-8")

            findings = check(root, "en")["findings"]
            codes = {item["code"] for item in findings}
            self.assertIn("catalog.duplicate", codes)
            self.assertIn("catalog.unpaired", codes)

    def test_unchanged_text_is_unicode_aware_but_ignores_language_variants(self) -> None:
        regional = check_pair("en", "Open Settings", "en-GB", "Open Settings")
        self.assertNotIn(
            "translation.unchanged",
            {item["code"] for item in regional["findings"]},
        )

        untranslated = check_pair(
            "bn",
            "আজকের পদ রচনা করা হচ্ছে",
            "gu",
            "আজকের পদ রচনা করা হচ্ছে",
        )
        self.assertIn(
            "translation.unchanged",
            {item["code"] for item in untranslated["findings"]},
        )


if __name__ == "__main__":
    unittest.main()
