#!/usr/bin/env python3
from __future__ import annotations

from collections.abc import Iterable
from pathlib import Path
import re
from urllib.request import urlopen


UNICODE_VERSION = "17.0.0"
CLDR_VERSION = "48.2"
UCD_BASE = f"https://www.unicode.org/Public/{UNICODE_VERSION}/ucd"
CLDR_LIKELY_SUBTAGS = (
    "https://raw.githubusercontent.com/unicode-org/cldr/"
    "release-48-2/common/supplemental/likelySubtags.xml"
)
OUTPUT = Path(__file__).resolve().parents[1] / "stringproof" / "unicode_data.py"


def download(url: str) -> str:
    with urlopen(url) as response:
        return response.read().decode("utf-8")


def records(text: str) -> Iterable[tuple[int, int, str, str]]:
    for raw_line in text.splitlines():
        line, _, comment = raw_line.partition("#")
        if ";" not in line:
            continue
        codepoints, value = (part.strip() for part in line.split(";", 1))
        bounds = codepoints.split("..")
        start = int(bounds[0], 16)
        end = int(bounds[-1], 16)
        category = comment.strip().split(maxsplit=1)[0]
        yield start, end, value, category


def script_aliases(text: str) -> tuple[dict[str, str], dict[str, str]]:
    short_by_name: dict[str, str] = {}
    long_by_short: dict[str, str] = {}
    for raw_line in text.splitlines():
        line = raw_line.partition("#")[0]
        fields = [field.strip() for field in line.split(";")]
        if len(fields) < 3 or fields[0] != "sc":
            continue
        short, long_name = fields[1], fields[2]
        short_by_name[short] = short
        short_by_name[long_name] = short
        long_by_short[short] = long_name.replace("_", " ")
    return short_by_name, long_by_short


def letter_ranges(
    scripts_text: str,
    extensions_text: str,
    aliases: dict[str, str],
    script_bits: dict[str, int],
) -> list[tuple[int, int, int, int]]:
    letters: dict[int, tuple[int, int]] = {}
    implicit = {"Common", "Inherited", "Unknown"}
    for start, end, script_name, category in records(scripts_text):
        if not category.startswith("L"):
            continue
        short = aliases[script_name]
        primary = 0 if script_name in implicit else script_bits[short]
        extensions = primary
        for codepoint in range(start, end + 1):
            letters[codepoint] = (primary, extensions)

    for start, end, script_names, category in records(extensions_text):
        if not category.startswith("L"):
            continue
        extensions = 0
        for name in script_names.split():
            extensions |= script_bits.get(aliases[name], 0)
        for codepoint in range(start, end + 1):
            if codepoint in letters:
                primary, _ = letters[codepoint]
                letters[codepoint] = (primary, extensions)

    compressed: list[tuple[int, int, int, int]] = []
    for codepoint, (primary, extensions) in sorted(letters.items()):
        if (
            compressed
            and compressed[-1][1] + 1 == codepoint
            and compressed[-1][2:] == (primary, extensions)
        ):
            start, _, previous_primary, previous_extensions = compressed[-1]
            compressed[-1] = (
                start,
                codepoint,
                previous_primary,
                previous_extensions,
            )
        else:
            compressed.append((codepoint, codepoint, primary, extensions))
    return compressed


def likely_scripts(text: str) -> dict[str, str]:
    result: dict[str, str] = {}
    pattern = re.compile(r'<likelySubtag\s+from="([^"]+)"\s+to="([^"]+)"')
    for source, target in pattern.findall(text):
        source_parts = source.replace("-", "_").split("_")
        target_parts = target.replace("-", "_").split("_")
        if len(target_parts) < 2:
            continue
        language = source_parts[0].lower()
        script = next(
            (
                part.title()
                for part in source_parts[1:]
                if len(part) == 4 and part.isalpha()
            ),
            None,
        )
        if script:
            continue
        region = next(
            (
                part.upper()
                for part in source_parts[1:]
                if (len(part) == 2 and part.isalpha())
                or (len(part) == 3 and part.isdigit())
            ),
            None,
        )
        key = f"{language}-{region}" if region else language
        result[key] = target_parts[1].title()
    return result


def tuple_literal(name: str, values: Iterable[int | str], width: int = 10) -> str:
    rendered = [repr(value) for value in values]
    lines = [
        "    " + ", ".join(rendered[index : index + width]) + ","
        for index in range(0, len(rendered), width)
    ]
    return f"{name} = (\n" + "\n".join(lines) + "\n)\n"


def dict_literal(name: str, values: dict[str, str]) -> str:
    lines = [f"    {key!r}: {values[key]!r}," for key in sorted(values)]
    return f"{name} = {{\n" + "\n".join(lines) + "\n}\n"


def main() -> None:
    aliases_text = download(f"{UCD_BASE}/PropertyValueAliases.txt")
    scripts_text = download(f"{UCD_BASE}/Scripts.txt")
    extensions_text = download(f"{UCD_BASE}/ScriptExtensions.txt")
    likely_text = download(CLDR_LIKELY_SUBTAGS)

    aliases, long_names = script_aliases(aliases_text)
    script_codes = sorted(
        short
        for short, long_name in long_names.items()
        if long_name not in {"Common", "Inherited", "Unknown"}
    )
    script_bits = {code: 1 << index for index, code in enumerate(script_codes)}
    ranges = letter_ranges(scripts_text, extensions_text, aliases, script_bits)

    output = [
        "# Generated by tools/generate_unicode_data.py; do not edit.\n",
        "# Derived from Unicode UCD and CLDR data under the Unicode License.\n",
        f"UNICODE_VERSION = {UNICODE_VERSION!r}\n",
        f"CLDR_VERSION = {CLDR_VERSION!r}\n\n",
        tuple_literal("SCRIPT_CODES", script_codes, 12),
        tuple_literal(
            "SCRIPT_NAMES",
            (long_names[code] for code in script_codes),
            6,
        ),
        tuple_literal("LETTER_STARTS", (item[0] for item in ranges)),
        tuple_literal("LETTER_ENDS", (item[1] for item in ranges)),
        tuple_literal("LETTER_PRIMARY", (item[2] for item in ranges), 6),
        tuple_literal("LETTER_EXTENSIONS", (item[3] for item in ranges), 4),
        dict_literal("LIKELY_SCRIPTS", likely_scripts(likely_text)),
    ]
    OUTPUT.write_text("".join(output), encoding="utf-8")


if __name__ == "__main__":
    main()
