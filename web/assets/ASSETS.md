# 鲸伴 Demo 素材说明

## 当前使用的鲸鱼娘表情图 whale-girl-sheet.png

来自用户在本次对话中提供的 4 × 4 表情图，780 × 761 像素。已原样复制，未生成重绘、未抠图、未修改源像素。界面通过 CSS 视窗显示相应表情，隐藏下方文字；使用 multiply 混合模式融合原图白色背景。

角色待机和聆听使用撒娇表情，思考使用省略号表情，回复使用开心表情，播放音乐使用欢呼表情。点击角色还可体验爱心、害羞、冲鸭和感谢。聊天头像和应用标志使用同一张原图。

旧的 whale.png 是早期生成的备用素材，目前界面不再使用。

## 原创小鲸鱼 `whale.png`

- 生成方式：Codex 内置 `image_gen` 工具，默认内置模式；未使用 CLI 或外部图片库。
- 这是为本项目生成的原创角色，未输入其他角色图片作为参考。
- PNG 包含原生透明通道；仅从默认生成目录复制到本目录，未对图片做任何后处理。
- 已目视检查：单只蓝色毛绒质感小鲸，白色肚皮、黑色亮眼睛、微笑、淡粉脸颊；完整身体，背景透明，无文字与道具。
- 生成时请求 1024×1024；实际工具返回尺寸以 PNG 自身元数据为准，前端按布局缩放显示。

生成 Prompt：

```text
Use case: stylized-concept
Asset type: transparent PNG mascot for an adult-friendly companion chat application
Primary request: Create one original charming blue baby whale companion character, rounded and cuddly, facing the viewer in a slight three-quarter view. Plump blue body, pale cream-white belly, small flippers, little curved tail visible, glossy black eyes, gentle closed smile, very subtle light pink cheeks. Friendly calm personality, premium polished 3D clay/plush illustration, soft tactile velvety material, tasteful and appealing to adults.
Scene/backdrop: genuinely transparent background, no environment.
Composition/framing: 1024 x 1024 square, full body centered, body occupies about 65% of canvas, transparent breathing room all around, no crop.
Lighting/mood: soft studio lighting, calm and comforting; subtle shading on the character only.
Color palette: periwinkle ocean blue, white belly, restrained blush.
Constraints: one character only, original design, no text, no letters, no logos, no watermark, no accessories, no props, no floor or drop-shadow plane. Preserve true transparency outside the character.
```

## 三首原创合成元音哼唱

| 文件 | 建议显示名称 | 时长 | 音频格式 |
| --- | --- | --- | --- |
| `tide.wav` | 潮汐来信 | 36 秒 | 24 kHz、双声道、16-bit PCM WAV |
| `moon.wav` | 月亮晚安 | 36 秒 | 24 kHz、双声道、16-bit PCM WAV |
| `cloud.wav` | 云朵散步 | 36 秒 | 24 kHz、双声道、16-bit PCM WAV |

这些是**程序合成的元音哼唱**，用于演示点歌、播放、进度、切歌和角色演唱状态。它们有原创的音高序列、节拍、元音共振峰、呼吸包络、颤音及轻柔和弦伴奏；不是纯单频提示音，也不是朗读歌词。

它们不是录音、不是实时 AI 创作歌曲，也不是专业歌声合成模型的输出。合成声音仍有明显电子音色，不能视作最终角色的自然人声。正式角色歌声与聊天声音的一致性，需要接入专业歌声模型或合规录制素材后替换。

旋律与合成脚本均为本次 Demo 编写，不使用现有歌曲、歌词或真人声音克隆。可通过 `tools/generate_audio.py` 重新生成，依赖 NumPy；随机种子固定。三首曲目均已验证文件可解码、持续时间 36 秒、波形非空且未削波。

本目录不包含任何 API 密钥。语音识别、聊天模型、语音合成的运行能力由主程序按实际配置提供，这些文件不表示已接入任何在线歌声服务。
