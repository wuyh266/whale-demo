# 语音资源发布与安装

本项目使用三种角色音色，每种由一份 GPT 权重和一份 SoVITS 权重组成。源码保存在 Git 中；本地 Python 环境、角色权重和公共推理资源单独打包，作为 GitHub Release 附件分发。仅下载 GitHub 自动生成的 Source code ZIP 不含这些资源。


### 发布者打包

先停止鲸伴和语音服务，安装 7-Zip。以下命令在包含 `START.cmd` 的项目根目录运行，示例版本名为 `v1`，发布新版本时更换包名。

```powershell
.\STOP.cmd
New-Item -ItemType Directory -Path .\dist -Force | Out-Null
& "$env:ProgramFiles\7-Zip\7z.exe" a -t7z -mx=1 -v1900m `
  ".\dist\whale-tts-backend-v1.7z" ".\backend" `
  '-xr!__pycache__' '-xr!*.pyc' '-xr!*.pyo' '-xr!*.log'
```

打包整个已裁剪的推理后端，包含 Python 环境、三个角色的 GPT/SoVITS 权重、公共推理模型、词典和组件许可证。归档中的路径从 `backend/gpt-sovits/` 开始，方便直接解压到源码根目录；不会包含工程根目录的 `.env` 或 `data/`。

输出示例：

```text
dist/
  whale-tts-backend-v1.7z.001
  whale-tts-backend-v1.7z.002
  whale-tts-backend-v1.7z.003
  ...
```

`dist/` 已由 Git 忽略，不需要 `git add -f`。打包完成后：

1. 将源码提交、推送到 GitHub。
2. 在仓库的 **Releases → Draft a new release** 中选择对应源码版本的标签。
3. 把上述所有 `.7z.001`、`.7z.002` 等文件作为附件上传，等全部上传完成后发布。
4. 在 Release 正文标明：这是 Windows 本地推理资源包、对应的源码版本、包含的三个音色，以及默认使用 CUDA。使用 CPU 时将 `config/sovits.json` 的 `device` 改为 `cpu`、`is_half` 改为 `false`。

源码与资源包需要匹配版本。资源包只包含后端；项目配置和三段参考音频已经随源码提供。

如果希望接收者不安装 Go，可另外编译 `bin/whale-demo.exe` 作为同一 Release 的附件，接收者将它放到项目 `bin/` 后使用 `START.cmd`。打包时保留组件随附许可证；角色权重和素材的分发遵循各自授权。

### 使用者安装

1. 下载该 Release 对应的源码 ZIP 并解压，或克隆对应标签。
2. 下载该版本全部资源分卷，放到同一个目录，保持 `.001`、`.002` 等文件名不变。
3. 用 7-Zip 打开第一卷 `.001`，解压到包含 `START.cmd` 的源码根目录。也可在源码根目录运行下面命令，把路径替换为实际下载位置：

```powershell
& "$env:ProgramFiles\7-Zip\7z.exe" x "下载目录\whale-tts-backend-v1.7z.001" '-o.'
```

解压后的目录应为：

```text
START.cmd
config/
  sovits.json
  reference/
backend/
  gpt-sovits/
    runtime/python.exe
    GPT_weights_v2ProPlus/
    SoVITS_weights_v2ProPlus/
    GPT_SoVITS/pretrained_models/
    GPT_SoVITS/text/G2PWModel/
    ...
```

不要解压成 `backend/gpt-sovits/backend/gpt-sovits/`。有新版源码时先使用与它对应的资源包。

首次运行将 `.env.example` 复制为 `.env`，按需填写 LLM 配置，并启用本地语音：

```dotenv
TTS_PROVIDER=gpt-sovits
GPT_SOVITS_API_BASE=http://127.0.0.1:9880
GPT_SOVITS_PROFILE=config/sovits.json
TTS_TIMEOUT_SECONDS=180
```

安装 Go 1.23 或更高版本，或放入 Release 提供的 `bin/whale-demo.exe`，然后双击 `START.cmd`。启动脚本会启动本地推理服务，页面的试听和朗读会向该服务请求合成。