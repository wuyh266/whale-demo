from pathlib import Path

root = Path(__file__).resolve().parent.parent
for name in ('web/index.html', 'web/app.js'):
    path = root / name
    text = path.read_text(encoding='utf-8')
    text = text.replace('/assets/whale.png', '/assets/whale-girl-sheet.png').replace('听它说话', '听她说话')
    path.write_text(text, encoding='utf-8')
path = root / 'provider.go'
text = path.read_text(encoding='utf-8').replace('一只温柔、有一点俏皮的蓝色鲸鱼伙伴', '一位蓝色长发、蓝色眼睛、带鲸鱼尾巴的Q版鲸鱼娘伙伴。你戴白色女仆发饰，性格温柔、有一点俏皮')
path.write_text(text, encoding='utf-8')
