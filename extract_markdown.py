#!/usr/bin/env python3
"""
从 PaddleOCR-VL JSON 中提取 block_content，生成纯净 Markdown。

用法：
    python3 extract_markdown.py
    python3 extract_markdown.py input.json output.md
"""

import json
import re
import sys
import os

# ---------- 配置 ----------
# 要跳过的 block_label（不输出也不产生空行）
SKIP_LABELS = {
    "image",
    "header_image",
    "footer_image",
    "aside_text",
    "number",
    "footnote",
    "vision_footnote",
    "header",
    "footer",
}

# 要保留为标题的 label → Markdown 标题层级映射
TITLE_LABELS = {
    "doc_title":          "# ",
    "paragraph_title":    "## ",
    "section_title":      "### ",
    "sub_section_title":  "#### ",
}


def clean_content(text: str) -> str:
    """清洗单条 block_content，去掉 HTML/图片/链接/LaTeX 等非文本格式。"""
    # 1. 去掉完整 HTML 标签（img, div, span 等）
    text = re.sub(r'<[^>]+>', '', text)

    # 2. 去掉图片 markdown 语法  ![alt](url)
    text = re.sub(r'!\[.*?\]\(.*?\)', '', text)

    # 3. 去掉链接 markdown 语法  [text](url) → 保留 text
    text = re.sub(r'\[([^\]]*?)\]\(.*?\)', r'\1', text)

    # 4. 去掉 LaTeX 数学模式  $...$  $$...$$
    text = re.sub(r'\$\$?[^$]*?\$\$?', '', text)

    # 5. 去掉上标/下标标记  ^{...}  _{...}
    text = re.sub(r'\^\{[^}]*\}', '', text)
    text = re.sub(r'_\{[^}]*\}', '', text)

    # 6. 去掉反斜杠转义字符
    text = text.replace('\\', '')

    # 7. 去掉多余空白（多个空格→单个，首尾去空格）
    text = re.sub(r'[ \t]+', ' ', text)
    text = text.strip()

    # 8. 合并 OCR 误识别的假段落分隔（\n\n 前后文本属于同一句子）
    text = _merge_false_paragraph_breaks(text)

    return text


def _merge_false_paragraph_breaks(text: str) -> str:
    """合并 OCR 误识别的假段落分隔符。

    当 \\n\\n 前后的文本属于同一句子时（前面不以句末标点结尾，
    后面以 CJK 字符开头），将分隔符去掉，文本直接拼接。
    真正的段落分隔（前面以句末标点结尾）保留 \\n\\n。
    """
    if '\n\n' not in text:
        return text

    SENTENCE_END = set('。！？…!?.\u2026')

    def _is_cjk(ch: str) -> bool:
        return '\u4e00' <= ch <= '\u9fff' or '\u3400' <= ch <= '\u4dbf'

    parts = text.split('\n\n')
    result = [parts[0]]

    for i in range(1, len(parts)):
        prev = result[-1].strip()
        curr = parts[i].strip()

        if prev and curr:
            last_char = prev[-1]
            first_char = curr[0]
            # 前段不以句末标点结尾 + 后段以 CJK 开头 → 假分段，直接拼接
            if last_char not in SENTENCE_END and _is_cjk(first_char):
                result[-1] = result[-1] + parts[i]
                continue

        result.append(parts[i])

    return '\n\n'.join(result)


def is_decorative(text: str) -> bool:
    """检测内容是否为装饰性符号（OCR 将 PDF 分隔线误识别为标题）。
    
    例如: ++++++, ------, ======, + + + + + 等。
    """
    compact = text.replace(' ', '')
    if len(compact) < 2:
        return False
    if len(set(compact)) == 1:
        ch = compact[0]
        # 只匹配非字母、非数字、非 CJK 字符的重复符号
        if not ch.isalnum() and not ('\u4e00' <= ch <= '\u9fff'):
            return True
    return False


# 句末标点集合（中文 + 英文）
_SENTENCE_END = set('。！？…!?.\u2026')


def ends_with_sentence_punct(text: str) -> bool:
    """判断文本是否以句末标点结尾。"""
    return bool(text) and text[-1] in _SENTENCE_END


def extract_markdown(input_path: str, output_path: str):
    with open(input_path, 'r', encoding='utf-8') as f:
        data = json.load(f)

    lines = []
    prev_was_empty = True  # 避免连续空行
    prev_was_text = False  # 上一个块是否为文本（用于连续文本合并）
    prev_ends_sentence = True  # 上一个文本块是否以句末标点结尾（初始 True 让第一个块独立成行）

    for page in data:
        pruned = page.get('prunedResult', {})
        blocks = pruned.get('parsing_res_list', [])

        for block in blocks:
            label = block.get('block_label', 'text')
            raw_content = block.get('block_content', '')

            # 跳过不需要的标签
            if label in SKIP_LABELS:
                continue

            content = clean_content(raw_content)
            if not content:
                continue

            # 根据 label 决定前缀
            prefix = TITLE_LABELS.get(label, '')

            # 去掉 content 开头多余的 # 号（OCR 常把 # 当作内容输出）
            if prefix:
                content = content.lstrip('#').lstrip()
                if not content:
                    continue

            # 跳过装饰性分隔线（OCR 将 PDF 分隔符误识别为标题或文本）
            # 先去掉可能残留的 # 再检测
            stripped = content.lstrip('#').strip()
            if is_decorative(stripped):
                continue

            line = f"{prefix}{content}"

            if prefix:
                # 标题：前加空行，后加空行
                if not prev_was_empty:
                    lines.append('')
                lines.append(line)
                lines.append('')
                prev_was_empty = True
                prev_was_text = False
            else:
                # 文本块：合并 OCR 导致的错误断行
                # 如果上一个块也是文本且未以句末标点结尾，将当前文本合并到上一行
                if prev_was_text and not prev_ends_sentence:
                    lines[-1] = lines[-1] + content
                else:
                    lines.append(content)
                prev_was_text = True
                prev_ends_sentence = ends_with_sentence_punct(content)
                prev_was_empty = False

    # 去掉尾部多余空行
    while lines and lines[-1] == '':
        lines.pop()
    while lines and lines[0] == '':
        lines.pop(0)

    result = '\n'.join(lines) + '\n'

    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(result)

    print(f"✓ 已生成: {output_path}")
    print(f"  共 {len(data)} 页, 输出 {len(result.splitlines())} 行")


if __name__ == '__main__':
    base_dir = os.path.dirname(os.path.abspath(__file__))

    if len(sys.argv) >= 3:
        input_path = sys.argv[1]
        output_path = sys.argv[2]
    elif len(sys.argv) == 2:
        input_path = sys.argv[1]
        output_path = os.path.splitext(input_path)[0] + '.md'
    else:
        input_path = os.path.join(
            base_dir, '永恒之光内文.pdf_by_PaddleOCR-VL-1.6.json')
        output_path = os.path.join(base_dir, '永恒之光内文_clean.md')

    extract_markdown(input_path, output_path)
