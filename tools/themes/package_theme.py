#!/usr/bin/env python3
"""Create a reproducible .s2theme ZIP from a theme.json manifest (stdlib only)."""
import argparse
import json
from pathlib import Path
import zipfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text(encoding='utf-8'))
    if not isinstance(manifest, dict) or manifest.get('schema_version') != 1:
        parser.error('manifest must be a theme API v1 JSON object')
    content = (json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + '\n').encode('utf-8')
    if len(content) > 32 * 1024:
        parser.error('theme.json must not exceed 32 KiB')
    args.output.parent.mkdir(parents=True, exist_ok=True)
    info = zipfile.ZipInfo('theme.json', date_time=(1980, 1, 1, 0, 0, 0))
    info.compress_type = zipfile.ZIP_DEFLATED
    info.create_system = 3
    info.external_attr = 0o100644 << 16
    with zipfile.ZipFile(args.output, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        archive.writestr(info, content)
    print(args.output)


if __name__ == '__main__':
    main()
