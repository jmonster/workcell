from __future__ import annotations

from bisect import bisect_right
from collections import Counter
from dataclasses import dataclass
from functools import lru_cache
import os
from pathlib import Path
import re
from typing import Iterable

from .unicode_data import (
    LETTER_ENDS,
    LETTER_EXTENSIONS,
    LETTER_PRIMARY,
    LETTER_STARTS,
    LIKELY_SCRIPTS,
    SCRIPT_CODES,
    SCRIPT_NAMES,
)


SCHEMA_VERSION = 1

PRINTF_PLACEHOLDER = re.compile(
    r"%(?:"
    r"%|"
    r"#@[A-Za-z_][A-Za-z0-9_.-]*@|"
    r"(?:\d+\$)?[-+#0 'I]*"
    r"(?:\*(?:\d+\$)?|\d+)?"
    r"(?:\.(?:\*(?:\d+\$)?|\d+))?"
    r"(?:hh|h|ll|l|q|L|j|z|t)?"
    r"[diuoxXfFeEgGaAcCsSpn@]"
    r")"
)
ICU_PLACEHOLDER = re.compile(r"\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?=[,}])")
ICU_SYNTAX = re.compile(
    r"\b(?:plural|selectordinal|select|offset|zero|one|two|few|many|other)\b"
)
SWIFT_PLACEHOLDER = re.compile(r"\\\([^)]*\)")

MOJIBAKE = re.compile(r"[ÃÂâ][\u0080-\u00BF]")
PROMPT_TRANSLATE_INTO = re.compile(
    r"\b(?:please\s+)?translate(?:\s+the\s+following)?"
    r"(?:\s+english\s+text)?\s+into\s+[a-z][a-z\s-]{1,30}:",
    re.IGNORECASE,
)
PROMPT_TEXT_INTO = re.compile(
    r"\b(?:english|source)\s+text\s+into\s+[a-z][a-z\s-]{1,30}:",
    re.IGNORECASE,
)
MODEL_TOKEN = re.compile(
    r"<(?:start|end)_of_turn>|<<<(?:source|target|text|source_language|"
    r"target_language|model)>>>|__(?:PH|LT)_\d{4}__|"
    r"\[{1,2}(?:TOK|PH|PLACEHOLDER)[_-]?\d+\]{1,2}",
    re.IGNORECASE,
)
PATH_TOKEN = re.compile(
    r"(?:^|[\s(\[\"'])(?:/|\.\.?/)[A-Za-z0-9][A-Za-z0-9._/-]{1,}"
)
SNAKE_TOKEN = re.compile(r"^[a-z0-9]+(?:_[a-z0-9]+)+$")
URL_OR_EMAIL = re.compile(
    r"(?:https?://\S+|mailto:\S+|[\w.+-]+@[\w.-]+\.[A-Za-z]{2,})",
    re.IGNORECASE,
)
MARKUP_TAG = re.compile(r"</?[A-Za-z][^>]*>")
FILENAME = re.compile(r"^(?![. ])[^/\\\r\n]*[^\s/\\]\.[A-Za-z0-9]{1,16}$")
IDENTIFIER = re.compile(r"^[^\W\d]\w*(?:[.:/-]\w+)*$", re.UNICODE)

STABLE_TERMS = frozenset(
    {
        "AI",
        "App Store",
        "Apple",
        "CloudKit",
        "JSON",
        "Lo-fi",
        "Mono",
        "PIN",
        "SSML",
        "Sans",
        "Serif",
        "Wi-Fi",
        "ZIP",
        "iCloud",
        "iOS",
        "iPhone",
    }
)
PROTECTED_LATIN_TERMS = STABLE_TERMS | {"App", "Fi", "ID", "Store", "Wi", "XIX"}
PROTECTED_TERM = re.compile(
    r"(?<![A-Za-z0-9])(?:"
    + "|".join(re.escape(term) for term in sorted(PROTECTED_LATIN_TERMS, key=len, reverse=True))
    + r")(?![A-Za-z0-9])"
)

SCRIPT_BITS = {code: 1 << index for index, code in enumerate(SCRIPT_CODES)}
SCRIPT_NAMES_BY_BIT = {
    1 << index: name.lower() for index, name in enumerate(SCRIPT_NAMES)
}
LATIN_BIT = SCRIPT_BITS["Latn"]
LATIN_CONNECTORS = {"a", "an", "and", "at", "by", "for", "in", "of", "on", "the", "to"}
SCRIPT_ALIASES: dict[str, tuple[str, ...]] = {
    "Aran": ("Arab",),
    "Cyrs": ("Cyrl",),
    "Geok": ("Geor",),
    "Hanb": ("Hani", "Bopo"),
    "Hans": ("Hani",),
    "Hant": ("Hani", "Bopo"),
    "Jpan": ("Hani", "Hira", "Kana", "Hrkt"),
    "Kore": ("Hang", "Hani"),
    "Latf": ("Latn",),
    "Latg": ("Latn",),
    "Syre": ("Syrc",),
    "Syrj": ("Syrc",),
    "Syrn": ("Syrc",),
}


class AuditError(Exception):
    """An input condition that prevents a trustworthy report."""


class ParseError(Exception):
    def __init__(self, line: int, message: str):
        super().__init__(message)
        self.line = line
        self.message = message


@dataclass(frozen=True)
class Entry:
    key: str
    value: str
    line: int


@dataclass
class Catalog:
    path: Path
    display_path: str
    locale: str
    logical_path: str
    entries: list[Entry]
    syntax_valid: bool = True

    @property
    def first_by_key(self) -> dict[str, Entry]:
        result: dict[str, Entry] = {}
        for entry in self.entries:
            result.setdefault(entry.key, entry)
        return result


@dataclass(frozen=True)
class Finding:
    code: str
    severity: str
    path: str
    line: int
    locale: str
    key: str
    message: str

    def as_dict(self) -> dict[str, object]:
        return {
            "code": self.code,
            "severity": self.severity,
            "path": self.path,
            "line": self.line,
            "locale": self.locale,
            "key": self.key,
            "message": self.message,
        }


class StringsParser:
    def __init__(self, text: str):
        self.text = text
        self.index = 0
        self.line = 1
        self.findings: list[tuple[int, str]] = []

    def parse(self) -> list[Entry]:
        entries: list[Entry] = []
        while True:
            try:
                self._skip_trivia()
                if self._at_end:
                    break
                line = self.line
                key = self._quoted("key")
                self._skip_trivia()
                self._expect("=", "expected '=' after key")
                self._skip_trivia()
                value = self._quoted("value")
                self._expect_semicolon()
                entries.append(Entry(key, value, line))
            except ParseError as error:
                self.findings.append((error.line, error.message))
                self._recover()
        return entries

    @property
    def _at_end(self) -> bool:
        return self.index >= len(self.text)

    def _advance(self) -> str:
        char = self.text[self.index]
        self.index += 1
        if char == "\n":
            self.line += 1
        return char

    def _skip_trivia(self) -> None:
        while not self._at_end:
            if self.text[self.index].isspace():
                self._advance()
            elif self.text.startswith("//", self.index):
                while not self._at_end and self._advance() != "\n":
                    pass
            elif self.text.startswith("/*", self.index):
                start_line = self.line
                self._advance()
                self._advance()
                while not self._at_end and not self.text.startswith("*/", self.index):
                    self._advance()
                if self._at_end:
                    raise ParseError(start_line, "unterminated block comment")
                self._advance()
                self._advance()
            else:
                return

    def _expect(self, expected: str, message: str) -> None:
        if self._at_end or self.text[self.index] != expected:
            raise ParseError(self.line, message)
        self._advance()

    def _expect_semicolon(self) -> None:
        start_index, start_line = self.index, self.line
        while not self._at_end and self.text[self.index].isspace():
            self._advance()
        if not self._at_end and self.text[self.index] == ";":
            self._advance()
            return
        self.index, self.line = start_index, start_line
        raise ParseError(start_line, "expected ';' after value")

    def _unicode_escape(self, label: str, start_line: int) -> str:
        digits = self.text[self.index : self.index + 4]
        if len(digits) != 4 or any(char not in "0123456789abcdefABCDEF" for char in digits):
            raise ParseError(start_line, f"invalid Unicode escape in {label}")
        for _ in range(4):
            self._advance()
        codepoint = int(digits, 16)
        if 0xD800 <= codepoint <= 0xDBFF and self.text.startswith("\\U", self.index):
            low_digits = self.text[self.index + 2 : self.index + 6]
            if len(low_digits) == 4 and all(
                char in "0123456789abcdefABCDEF" for char in low_digits
            ):
                low = int(low_digits, 16)
                if 0xDC00 <= low <= 0xDFFF:
                    for _ in range(6):
                        self._advance()
                    codepoint = 0x10000 + ((codepoint - 0xD800) << 10) + low - 0xDC00
        if 0xD800 <= codepoint <= 0xDFFF:
            raise ParseError(start_line, f"isolated surrogate escape in {label}")
        return chr(codepoint)

    def _quoted(self, label: str) -> str:
        if self._at_end or self.text[self.index] != '"':
            raise ParseError(self.line, f"expected quoted {label}")
        start_line = self.line
        self._advance()
        output: list[str] = []
        escapes = {
            "b": "\b",
            "f": "\f",
            "n": "\n",
            "r": "\r",
            "t": "\t",
            '"': '"',
            "\\": "\\",
        }
        while not self._at_end:
            char = self._advance()
            if char == '"':
                return "".join(output)
            if char in "\r\n":
                raise ParseError(start_line, f"unterminated quoted {label}")
            if char != "\\":
                output.append(char)
                continue
            if self._at_end:
                raise ParseError(start_line, f"unterminated escape in {label}")
            escaped = self._advance()
            if escaped in {"U", "u"}:
                output.append(self._unicode_escape(label, start_line))
            else:
                output.append(escapes.get(escaped, escaped))
        raise ParseError(start_line, f"unterminated quoted {label}")

    def _recover(self) -> None:
        while not self._at_end:
            if self._advance() in ";\n":
                return


def _normalize_locale(locale: str) -> str:
    return locale.strip().replace("_", "-")


def _locale_parts(locale: str) -> tuple[str, str | None, str | None]:
    parts = [part for part in _normalize_locale(locale).split("-") if part]
    if not parts:
        return "", None, None
    language = parts[0].lower()
    script: str | None = None
    region: str | None = None
    for part in parts[1:]:
        if len(part) == 1 and part.isalnum():
            break
        if script is None and len(part) == 4 and part.isalpha():
            script = part.title()
        elif region is None and (
            (len(part) == 2 and part.isalpha())
            or (len(part) == 3 and part.isdigit())
        ):
            region = part.upper()
    return language, script, region


def _script_mask(script: str) -> int:
    direct = SCRIPT_BITS.get(script)
    if direct:
        return direct
    mask = 0
    for component in SCRIPT_ALIASES.get(script, ()):
        mask |= SCRIPT_BITS.get(component, 0)
    return mask


@lru_cache(maxsize=512)
def _expected_script_mask(locale: str) -> int:
    language, explicit_script, region = _locale_parts(locale)
    if explicit_script:
        return _script_mask(explicit_script)
    script = None
    if language and region:
        script = LIKELY_SCRIPTS.get(f"{language}-{region}")
    if script is None and language:
        script = LIKELY_SCRIPTS.get(language)
    return _script_mask(script) if script else 0


@lru_cache(maxsize=8192)
def _letter_scripts(char: str) -> tuple[int, int]:
    codepoint = ord(char)
    index = bisect_right(LETTER_STARTS, codepoint) - 1
    if index >= 0 and codepoint <= LETTER_ENDS[index]:
        return LETTER_PRIMARY[index], LETTER_EXTENSIONS[index]
    return 0, 0


def _decode_catalog(path: Path) -> str:
    try:
        raw = path.read_bytes()
    except OSError as error:
        raise AuditError(f"cannot read {path}: {error}") from error
    try:
        if raw.startswith(b"\xef\xbb\xbf"):
            return raw.decode("utf-8-sig")
        if raw.startswith((b"\xff\xfe", b"\xfe\xff")):
            return raw.decode("utf-16")
        return raw.decode("utf-8")
    except UnicodeDecodeError as utf8_error:
        if raw and len(raw) % 2 == 0:
            pairs = len(raw) // 2
            encodings: list[str] = []
            if raw[1::2].count(0) / pairs >= 0.15:
                encodings.append("utf-16-le")
            if raw[0::2].count(0) / pairs >= 0.15:
                encodings.append("utf-16-be")
            for encoding in encodings:
                try:
                    return raw.decode(encoding)
                except UnicodeDecodeError:
                    pass
        raise AuditError(f"cannot decode {path} as UTF-8 or UTF-16") from utf8_error


def _logical_path(path: Path, root: Path) -> str:
    parts = list(path.relative_to(root).parts)
    for index, part in enumerate(parts):
        if part.endswith(".lproj"):
            parts[index] = "{locale}.lproj"
            break
    return "/".join(parts)


def _finding(
    code: str,
    severity: str,
    catalog: Catalog,
    line: int,
    key: str,
    message: str,
) -> Finding:
    return Finding(
        code,
        severity,
        catalog.display_path,
        max(1, line),
        catalog.locale,
        key or "<catalog>",
        message,
    )


def parse_catalog(path: Path, root: Path) -> tuple[Catalog, list[Finding]]:
    catalog = Catalog(
        path=path,
        display_path=path.relative_to(root).as_posix(),
        locale=_normalize_locale(path.parent.name[:-6]),
        logical_path=_logical_path(path, root),
        entries=[],
    )
    parser = StringsParser(_decode_catalog(path))
    catalog.entries = parser.parse()
    catalog.syntax_valid = not parser.findings
    findings = [
        _finding("syntax.invalid", "error", catalog, line, "<catalog>", message)
        for line, message in parser.findings
    ]
    first_lines: dict[str, int] = {}
    for entry in catalog.entries:
        if entry.key in first_lines:
            findings.append(
                _finding(
                    "key.duplicate",
                    "error",
                    catalog,
                    entry.line,
                    entry.key,
                    f"duplicate key; first declared on line {first_lines[entry.key]}",
                )
            )
        else:
            first_lines[entry.key] = entry.line
    return catalog, findings


def extract_placeholders(text: str) -> list[str]:
    tokens = PRINTF_PLACEHOLDER.findall(text)
    tokens.extend(f"{{{match.group(1)}}}" for match in ICU_PLACEHOLDER.finditer(text))
    tokens.extend(SWIFT_PLACEHOLDER.findall(text))
    return sorted(tokens)


def _format_tokens(tokens: Iterable[str]) -> str:
    counts = Counter(tokens)
    return ", ".join(
        f"{token} ×{count}" if count > 1 else token
        for token, count in sorted(counts.items())
    )


def _placeholder_findings(
    source: Entry, target: Entry, catalog: Catalog
) -> list[Finding]:
    source_tokens = Counter(extract_placeholders(source.value))
    target_tokens = Counter(extract_placeholders(target.value))
    missing = list((source_tokens - target_tokens).elements())
    added = list((target_tokens - source_tokens).elements())
    findings: list[Finding] = []
    if missing:
        findings.append(
            _finding(
                "placeholder.missing",
                "error",
                catalog,
                target.line,
                target.key,
                f"missing placeholder(s): {_format_tokens(missing)}",
            )
        )
    if added:
        findings.append(
            _finding(
                "placeholder.added",
                "error",
                catalog,
                target.line,
                target.key,
                f"added placeholder(s): {_format_tokens(added)}",
            )
        )
    return findings


def _encoding_findings(entry: Entry, catalog: Catalog) -> list[Finding]:
    findings: list[Finding] = []
    if "\ufffd" in entry.value:
        findings.append(
            _finding(
                "encoding.replacement",
                "error",
                catalog,
                entry.line,
                entry.key,
                "contains Unicode replacement character U+FFFD",
            )
        )
    indicators = sorted(set(MOJIBAKE.findall(entry.value)))
    controls = sorted(
        {f"U+{ord(char):04X}" for char in entry.value if 0x80 <= ord(char) <= 0x9F}
    )
    if indicators or controls:
        findings.append(
            _finding(
                "encoding.mojibake",
                "error",
                catalog,
                entry.line,
                entry.key,
                f"contains strong mojibake indicator(s): {', '.join(indicators + controls)}",
            )
        )
    return findings


def _source_allows_path(source: str) -> bool:
    value = source.strip()
    return (
        "://" in value
        or bool(FILENAME.fullmatch(value))
        or bool(PATH_TOKEN.search(value))
        or bool(SNAKE_TOKEN.fullmatch(value.lower()))
    )


def _model_residue(entry: Entry, source: Entry, catalog: Catalog) -> Finding | None:
    value = entry.value.strip()
    lower = value.lower()
    markers = (
        "please translate the following",
        "english text into",
        "<start_of_turn>",
        "<end_of_turn>",
        "<<<source>>>",
        "<<<target>>>",
        "<<<text>>>",
        "<<<source_language>>>",
        "<<<target_language>>>",
        "<<<model>>>",
    )
    prompt = (
        lower in {"model", ".", ":", ";", "-", "word."}
        or any(marker in lower for marker in markers)
        or bool(PROMPT_TRANSLATE_INTO.search(value))
        or bool(PROMPT_TEXT_INTO.search(value))
        or bool(MODEL_TOKEN.search(value))
    )
    path_leak = not _source_allows_path(source.value) and bool(
        SNAKE_TOKEN.fullmatch(lower) or PATH_TOKEN.search(value)
    )
    if not prompt and not path_leak:
        return None
    detail = (
        "model-like slug or path residue"
        if path_leak and not prompt
        else "translation prompt or model-control residue"
    )
    return _finding(
        "model.residue",
        "error",
        catalog,
        entry.line,
        entry.key,
        f"contains {detail}",
    )


def _is_single_proper_noun(text: str) -> bool:
    value = text.strip()
    letters = [char for char in value if char.isalpha()]
    return bool(
        letters
        and letters[0].isupper()
        and all(char.isalpha() or char in "'’-." for char in value)
    )


def _blank(pattern: re.Pattern[str], text: str) -> str:
    return pattern.sub(lambda match: " " * len(match.group()), text)


def _script_content(entry: Entry, source: Entry) -> str:
    value = entry.value
    if FILENAME.fullmatch(value.strip()):
        return ""
    if (
        value.strip() == source.value.strip()
        and _is_non_lexical_source(source.value, source.key)
    ):
        return ""
    for pattern in (
        URL_OR_EMAIL,
        MARKUP_TAG,
        PRINTF_PLACEHOLDER,
        SWIFT_PLACEHOLDER,
        ICU_PLACEHOLDER,
        ICU_SYNTAX,
        PROTECTED_TERM,
    ):
        value = _blank(pattern, value)
    if _is_single_proper_noun(source.value):
        value = value.replace(source.value.strip(), " ")
    return value


def _unexpected_scripts(value: str, allowed: int) -> list[tuple[int, str]]:
    counts: Counter[int] = Counter()
    samples: dict[int, list[str]] = {}
    mixed: set[int] = set()
    for index, char in enumerate(value):
        primary, extensions = _letter_scripts(char)
        if not primary or extensions & allowed:
            continue
        counts[primary] += 1
        sample = samples.setdefault(primary, [])
        if len(sample) < 16:
            sample.append(char)
        for neighbor_index in (index - 1, index + 1):
            if 0 <= neighbor_index < len(value) and value[neighbor_index].isalpha():
                _, neighbor_extensions = _letter_scripts(value[neighbor_index])
                if neighbor_extensions & allowed:
                    mixed.add(primary)
    return [
        (script, "".join(samples[script]))
        for script, count in counts.items()
        if count >= 2 or script in mixed
    ]


def _latin_runs(value: str) -> list[tuple[str, list[str]]]:
    runs: list[tuple[str, list[str]]] = []
    characters: list[str] = []
    tokens: list[str] = []
    token: list[str] = []

    def finish_token() -> None:
        if token:
            tokens.append("".join(token))
            token.clear()

    def finish_run() -> None:
        finish_token()
        if tokens:
            runs.append(("".join(characters).strip(), list(tokens)))
        characters.clear()
        tokens.clear()

    for char in value:
        primary, _ = _letter_scripts(char)
        if primary == LATIN_BIT:
            characters.append(char)
            token.append(char)
        elif characters and char in " -'’":
            characters.append(char)
            finish_token()
        else:
            finish_run()
    finish_run()
    return runs


def _latin_residue(value: str, expected: int) -> str | None:
    if expected & LATIN_BIT:
        return None
    for fragment, tokens in _latin_runs(value):
        if any(token.isupper() and len(token) >= 4 for token in tokens):
            return fragment
        lexical = [
            token
            for token in tokens
            if token.casefold() not in LATIN_CONNECTORS
            and not token[:1].isupper()
            and len(token) >= 2
        ]
        if len(lexical) >= 2:
            return fragment
    return None


def _script_finding(
    entry: Entry, source: Entry, catalog: Catalog
) -> Finding | None:
    allowed = _expected_script_mask(catalog.locale)
    if not allowed:
        return None
    value = _script_content(entry, source)
    unexpected = _unexpected_scripts(value, allowed | LATIN_BIT)
    latin = _latin_residue(value, allowed)
    if not unexpected and not latin:
        return None
    unexpected.sort(key=lambda item: SCRIPT_NAMES_BY_BIT.get(item[0], ""))
    details = [
        f'{SCRIPT_NAMES_BY_BIT.get(script, "unknown")} ({sample!r})'
        for script, sample in unexpected
    ]
    if latin:
        details.append(f"latin ({latin!r})")
    return _finding(
        "script.unexpected",
        "warning",
        catalog,
        entry.line,
        entry.key,
        f"unexpected script(s): {', '.join(details)}",
    )


def _is_non_lexical_source(source: str, key: str) -> bool:
    value = source.strip()
    if not value or value in STABLE_TERMS:
        return True
    if URL_OR_EMAIL.fullmatch(value) or FILENAME.fullmatch(value):
        return True
    if key.startswith("speech.") and IDENTIFIER.fullmatch(value):
        return True
    if re.fullmatch(r"[A-Z0-9_.-]{2,10}", value):
        return True
    masked = value
    for pattern in (
        PRINTF_PLACEHOLDER,
        SWIFT_PLACEHOLDER,
        ICU_PLACEHOLDER,
        ICU_SYNTAX,
    ):
        masked = _blank(pattern, masked)
    letters = sum(char.isalpha() for char in masked)
    return letters == 0 or (letters == 1 and bool(extract_placeholders(value)))


def _same_language(source_locale: str, target_locale: str) -> bool:
    source_language, _, _ = _locale_parts(source_locale)
    target_language, _, _ = _locale_parts(target_locale)
    return bool(source_language and source_language == target_language)


def _content_findings(
    source: Entry,
    target: Entry,
    source_locale: str,
    catalog: Catalog,
) -> list[Finding]:
    findings = _placeholder_findings(source, target, catalog)
    findings.extend(_encoding_findings(target, catalog))
    residue = _model_residue(target, source, catalog)
    if residue:
        findings.append(residue)
    script = _script_finding(target, source, catalog)
    if script:
        findings.append(script)
    if (
        not _same_language(source_locale, catalog.locale)
        and source.value.strip()
        and source.value.strip() == target.value.strip()
        and not _is_non_lexical_source(source.value, source.key)
    ):
        findings.append(
            _finding(
                "translation.unchanged",
                "warning",
                catalog,
                target.line,
                target.key,
                "matches lexical source text and may be untranslated",
            )
        )
    return findings


def _discover_catalogs(root: Path) -> list[Path]:
    try:
        paths = [
            path
            for path in root.rglob("*.strings")
            if path.is_file() and path.parent.name.endswith(".lproj")
        ]
    except OSError as error:
        raise AuditError(f"cannot discover catalogs under {root}: {error}") from error
    return sorted(paths, key=lambda path: path.relative_to(root).as_posix())


def check(root: Path, source_locale: str) -> dict[str, object]:
    try:
        resolved = root.expanduser().resolve(strict=True)
    except OSError as error:
        raise AuditError(f"cannot read root {root}: {error}") from error
    if not resolved.is_dir():
        raise AuditError(f"root is not a directory: {resolved}")
    if not os.access(resolved, os.R_OK | os.X_OK):
        raise AuditError(f"root is not readable: {resolved}")

    paths = _discover_catalogs(resolved)
    if not paths:
        raise AuditError(f"no <locale>.lproj/*.strings catalogs found under {resolved}")

    catalogs: list[Catalog] = []
    findings: list[Finding] = []
    for path in paths:
        catalog, catalog_findings = parse_catalog(path, resolved)
        catalogs.append(catalog)
        findings.extend(catalog_findings)

    normalized_source = _normalize_locale(source_locale).lower()
    if not any(catalog.locale.lower() == normalized_source for catalog in catalogs):
        raise AuditError(
            f"source locale {source_locale!r} has no .strings catalogs under {resolved}"
        )

    by_logical: dict[str, dict[str, Catalog]] = {}
    for catalog in catalogs:
        localized = by_logical.setdefault(catalog.logical_path, {})
        locale_key = catalog.locale.lower()
        existing = localized.get(locale_key)
        if existing:
            findings.append(
                _finding(
                    "catalog.duplicate",
                    "error",
                    catalog,
                    1,
                    "<catalog>",
                    f"duplicates locale catalog {existing.display_path}",
                )
            )
        else:
            localized[locale_key] = catalog

    for logical_path in sorted(by_logical):
        localized = by_logical[logical_path]
        source_catalog = localized.get(normalized_source)
        if source_catalog is None:
            for target_catalog in localized.values():
                findings.append(
                    _finding(
                        "catalog.unpaired",
                        "error",
                        target_catalog,
                        1,
                        "<catalog>",
                        f"target catalog has no {source_locale} source catalog at the same relative path",
                    )
                )
            continue
        if not source_catalog.syntax_valid:
            continue

        source_entries = source_catalog.first_by_key
        for locale, target_catalog in sorted(localized.items()):
            if locale == normalized_source or not target_catalog.syntax_valid:
                continue
            target_entries = target_catalog.first_by_key
            for key in source_entries.keys() - target_entries.keys():
                findings.append(
                    _finding(
                        "key.missing",
                        "error",
                        target_catalog,
                        1,
                        key,
                        "key is present in source catalog but missing from target",
                    )
                )
            for key in target_entries.keys() - source_entries.keys():
                target = target_entries[key]
                findings.append(
                    _finding(
                        "key.unexpected",
                        "error",
                        target_catalog,
                        target.line,
                        key,
                        "key is absent from source catalog",
                    )
                )
            for key in source_entries.keys() & target_entries.keys():
                findings.extend(
                    _content_findings(
                        source_entries[key],
                        target_entries[key],
                        source_catalog.locale,
                        target_catalog,
                    )
                )

    findings.sort(
        key=lambda item: (
            item.path,
            item.line,
            item.code,
            item.locale,
            item.key,
            item.message,
        )
    )
    by_code = Counter(finding.code for finding in findings)
    return {
        "schema_version": SCHEMA_VERSION,
        "status": "findings" if findings else "clean",
        "findings": [finding.as_dict() for finding in findings],
        "summary": {
            "files": len(catalogs),
            "locales": len({catalog.locale for catalog in catalogs}),
            "findings": len(findings),
            "errors": sum(finding.severity == "error" for finding in findings),
            "warnings": sum(finding.severity == "warning" for finding in findings),
            "by_code": dict(sorted(by_code.items())),
        },
    }
