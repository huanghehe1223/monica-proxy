from pathlib import Path
import requests


BASE_URL = "https://huanghe1223-terminal.ms.fun"
DRAWIO_FILE = Path("test_drawio_export/template.drawio")
OUTPUT_FILE = Path("test_drawio_export/template.png")


def export_drawio_to_png():
    if not DRAWIO_FILE.exists():
        raise FileNotFoundError(f"找不到文件: {DRAWIO_FILE}")

    xml = DRAWIO_FILE.read_text(encoding="utf-8")

    url = f"{BASE_URL.rstrip('/')}/export"

    response = requests.post(
        url,
        data={
            "format": "png",
            "xml": xml,
        },
        timeout=120,
    )

    print("Status code:", response.status_code)
    print("Content-Type:", response.headers.get("content-type"))

    if not response.ok:
        print("Response text:")
        print(response.text[:2000])
        response.raise_for_status()

    # 简单校验是否为 PNG
    if not response.content.startswith(b"\x89PNG\r\n\x1a\n"):
        print("Warning: 返回内容不像 PNG 文件")
        print(response.content[:500])
        raise RuntimeError("Export failed: response is not a PNG")

    OUTPUT_FILE.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT_FILE.write_bytes(response.content)

    print(f"导出成功: {OUTPUT_FILE}")
    print(f"文件大小: {OUTPUT_FILE.stat().st_size / 1024:.2f} KB")


if __name__ == "__main__":
    export_drawio_to_png()