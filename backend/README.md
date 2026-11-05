# 本地语音推理后端

`gpt-sovits/` 保存鲸伴使用的 GPT-SoVITS 推理源码，不包含训练入口。主程序通过 `../tools/sovits_server.py` 提供本地语音接口；启动入口为项目根目录的 `START-SOVITS.cmd`，`START.cmd` 也可按配置同时启动语音服务。

Git 仓库不包含完整运行环境。使用本地推理时，需要在 `gpt-sovits/` 内补齐以下资源：

| 路径 | 内容 |
| --- | --- |
| `runtime/` | Windows Python、PyTorch/CUDA、音频处理依赖、FFmpeg、日语基础词典及 NLTK 英文语言资源 |
| `GPT_weights_v2ProPlus/` | 三种音色的 GPT 权重 |
| `SoVITS_weights_v2ProPlus/` | 与 GPT 权重配套的 SoVITS 权重 |
| `GPT_SoVITS/pretrained_models/` | 中文 BERT、HuBERT、语言识别、说话人特征等预训练模型 |
| `GPT_SoVITS/text/G2PWModel/` | 中文多音字模型 |
| `GPT_SoVITS/text/ja_userdic/user.dict` | 日语用户编译词典，Git 忽略但本地启动必需；源数据 `userdict.csv` 随源码提交 |
| `GPT_SoVITS/text/` 等语言处理目录 | 分词、发音所需的字典、模型与大型语言资源 |

具体模型文件、参考音频及参数以 `../config/sovits.json` 为准。目前配置千早爱音、长崎素世和客服小祥三种音色；参考音频保存在项目的 `config/reference/`，全部使用项目内相对路径。

运行环境、权重、大型语料和缓存不提交 GitHub，本地文件可保留。源码仓库需要补齐这些资源后才能进行本地推理；复制可运行环境时应连同这些本地资源一起复制。

`GPT_SoVITS/pretrained_models/` 是三个角色共用的推理依赖，当前版本不能删除。公共模型的用途，以及将后端分卷上传 GitHub Releases、供其他人下载安装的步骤，见 [语音资源发布与安装](../docs/RELEASING.md)。

上游许可证见 `gpt-sovits/LICENSE`，各依赖的许可说明随对应组件保留。
