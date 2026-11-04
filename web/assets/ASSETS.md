# 素材说明

## 角色图片

| 文件 | 来源与用途 |
| --- | --- |
| `whale-girl-sheet.png` | 用户提供的 4 × 4 表情图，780 × 761 像素；用于大肥鱼的表情、头像和应用标志，网页通过 CSS 视窗显示相应表情。 |
| `anon.png` | 用户提供的千早爱音参考图，942 × 1670 像素；用于头像和参考图预览。 |
| `anon-poster.png` | 用户提供的角色海报；保留透明通道，叠放在爱音主题背景上。 |

上述图片保留用户提供文件的像素内容。自定义伴侣上传的图片保存在 `data/uploads/`，不属于内置素材。

README 使用 `anon.png` 展示角色参考图；`docs/images/whale-demo.png` 是用户提供的实际运行截图，仅供项目文档展示。

## 音乐

`tide.wav`、`moon.wav`、`cloud.wav` 是为本项目编写的三段原创合成元音哼唱，每段 36 秒，格式为 24 kHz、双声道、16-bit PCM WAV。它们采用程序合成的旋律与伴奏，有电子音色；不是角色录制的歌声，也不是实时生成的歌曲。

## 语音参考

以下日语参考音频由用户提供，推理配置与对应原文保存在 `config/sovits.json`：

| 文件 | 音色 |
| --- | --- |
| `config/reference/anon.wav` | 千早爱音 |
| `config/reference/soyo.wav` | 长崎素世 |
| `config/reference/sakiko-white.wav` | 客服小祥 |

三段音频均为 32 kHz、单声道 PCM16 WAV。本说明仅记录来源和用途，不代表角色图片、参考音频或模型权重的分发授权。
