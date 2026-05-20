from pathlib import Path
import shutil

# 源目录
source_dir = Path("/root/.codex")

# 输出目录
output_dir = Path("/kaggle/working")
output_dir.mkdir(parents=True, exist_ok=True)

# 输出压缩包路径，不需要写 .zip 后缀
output_base = output_dir / "codex_backup"

# 检查源目录是否存在
if not source_dir.exists():
    raise FileNotFoundError(f"源目录不存在: {source_dir}")

# 压缩为 zip
zip_path = shutil.make_archive(
    base_name=str(output_base),
    format="zip",
    root_dir=str(source_dir.parent),
    base_dir=source_dir.name
)

print(f"压缩完成: {zip_path}")