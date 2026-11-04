"""Local GPT-SoVITS bridge. Model/source files are read from source_dir only.

Run with the bundled backend/gpt-sovits/runtime Python environment. All
generated configuration and caches belong to whale/data/sovits-runtime.
"""

import argparse
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import sys
import tempfile
import threading
import traceback


APP_ROOT = Path(__file__).resolve().parents[1]
VERSIONS = {"v1", "v2", "v2Pro", "v2ProPlus", "v3", "v4"}
LANGUAGES = {"auto", "auto_yue", "zh", "ja", "en", "ko", "yue", "all_zh", "all_ja", "all_ko", "all_yue"}
VOICE_ID = re.compile(r"^[a-z][a-z0-9_]{0,31}$")
VOICE_FIELDS = {"name", "version", "gpt_weights", "sovits_weights", "ref_audio_path", "prompt_text", "prompt_lang", "text_lang", "text_split_method", "speed_factor"}


def within(path, root=None):
    try:
        Path(path).resolve().relative_to((root or APP_ROOT).resolve())
        return True
    except (ValueError, TypeError, OSError):
        return False


def demo_path(value, base=None, label="资源路径"):
    if not isinstance(value, str) or not value.strip():
        raise ValueError(label + " 不能为空")
    path = Path(value)
    if not path.is_absolute():
        path = (base or APP_ROOT) / path
    path = path.resolve()
    if not within(path):
        raise ValueError(label + " 必须位于 whale-demo 文件夹内：" + str(path))
    return path


def source_path(profile, key):
    value = profile.get(key, "")
    if not isinstance(value, str) or not value.strip():
        return None
    # Reference audio travels with the app; model/runtime paths belong to the
    # backend. Neither depends on the current directory or an original drive.
    base = APP_ROOT if key == "ref_audio_path" else Path(profile["source_dir"])
    return demo_path(value, base=base, label=key)


def validate_profile_values(profile):
    for key in ("python", "gpt_weights", "sovits_weights", "ref_audio_path"):
        source_path(profile, key)
    if profile.get("version") not in VERSIONS:
        raise ValueError("不支持的 GPT-SoVITS version")
    if profile.get("device") not in {"cpu", "cuda", "cuda:0"}:
        raise ValueError("device 请填写 cpu、cuda 或 cuda:0")
    if not isinstance(profile.get("is_half"), bool):
        raise ValueError("is_half 必须是 true 或 false")
    for key in ("text_lang", "prompt_lang"):
        if profile.get(key) not in LANGUAGES:
            raise ValueError(key + " 不是支持的语言代码")
    speed = profile.get("speed_factor", 1.0)
    if isinstance(speed, bool) or not isinstance(speed, (int, float)) or not 0.5 <= speed <= 2.0:
        raise ValueError("speed_factor 应在 0.5 到 2.0 之间")
    if not isinstance(profile.get("prompt_text", ""), str):
        raise ValueError("prompt_text 必须是参考音频的实际文字")
    if "name" in profile and not isinstance(profile["name"], str):
        raise ValueError("音色名称必须是文字")


def voice_catalog(profile):
    """Expand voice overrides without allowing a voice to move the backend."""
    base = {key: value for key, value in profile.items() if key not in {"voices", "default_voice"}}
    voices = profile.get("voices")
    if voices is None:
        voice_id = profile.get("_voice_id", "anon")
        return {voice_id: base}
    if not isinstance(voices, dict) or not voices:
        raise ValueError("voices 必须是非空音色配置对象")
    catalog = {}
    for voice_id, overrides in voices.items():
        if not isinstance(voice_id, str) or not VOICE_ID.fullmatch(voice_id) or not isinstance(overrides, dict):
            raise ValueError("音色 ID 或配置无效")
        if set(overrides) - VOICE_FIELDS:
            raise ValueError("音色只能覆盖模型、参考音频和合成参数，不能更改运行目录")
        selected = dict(base)
        selected.update(overrides)
        selected["_voice_id"] = voice_id
        selected["_catalog_voice"] = True
        catalog[voice_id] = selected
    return catalog


def default_voice_id(profile):
    return profile.get("default_voice", profile.get("_voice_id", "anon"))


def select_voice(profile, voice_id=None):
    voice_id = default_voice_id(profile) if voice_id is None else voice_id
    if not isinstance(voice_id, str) or voice_id not in voice_catalog(profile):
        raise ValueError("未配置该音色，请选择已提供的声音")
    return voice_id, voice_catalog(profile)[voice_id]


def public_voices(profile):
    return [{"id": voice_id, "name": selected.get("name", "千早爱音" if voice_id == "anon" else voice_id)}
            for voice_id, selected in voice_catalog(profile).items()]


def read_profile(path):
    path = demo_path(str(path), label="配置文件")
    with path.open("r", encoding="utf-8-sig") as stream:
        profile = json.load(stream)
    if not isinstance(profile, dict):
        raise ValueError("配置必须是 JSON 对象")
    profile["source_dir"] = str(demo_path(profile.get("source_dir"), label="source_dir"))
    validate_profile_values(profile)
    for selected in voice_catalog(profile).values():
        validate_profile_values(selected)
    select_voice(profile)
    return profile


def single_voice_resources(profile, require_reference=True):
    source = Path(profile["source_dir"])
    required = {
        "推理源码": source / "GPT_SoVITS/TTS_infer_pack/TTS.py",
        "Python 运行环境": source_path(profile, "python"),
        "GPT 权重": source_path(profile, "gpt_weights"),
        "SoVITS 权重": source_path(profile, "sovits_weights"),
        "中文 BERT 配置": source / "GPT_SoVITS/pretrained_models/chinese-roberta-wwm-ext-large/config.json",
        "中文 BERT 权重": source / "GPT_SoVITS/pretrained_models/chinese-roberta-wwm-ext-large/pytorch_model.bin",
        "中文 BERT 分词器": source / "GPT_SoVITS/pretrained_models/chinese-roberta-wwm-ext-large/tokenizer.json",
        "HuBERT 配置": source / "GPT_SoVITS/pretrained_models/chinese-hubert-base/config.json",
        "HuBERT 权重": source / "GPT_SoVITS/pretrained_models/chinese-hubert-base/pytorch_model.bin",
        "中文多音字模型": source / "GPT_SoVITS/text/G2PWModel/g2pW.onnx",
        "语言识别模型": source / "GPT_SoVITS/pretrained_models/fast_langdetect/lid.176.bin",
        "日语用户词典": source / "GPT_SoVITS/text/ja_userdic/user.dict",
        "日语基础词典": source / "runtime/Lib/site-packages/pyopenjtalk/open_jtalk_dic_utf_8-1.11/sys.dic",
        "英文发音词典": source / "runtime/nltk_data/corpora/cmudict/cmudict",
        "英文词性模型": source / "runtime/nltk_data/taggers/averaged_perceptron_tagger/averaged_perceptron_tagger.pickle",
        "音频解码器": source / "runtime/ffmpeg.exe",
        "音频信息工具": source / "runtime/ffprobe.exe",
    }
    if profile["version"] in {"v2Pro", "v2ProPlus"}:
        required["说话人模型"] = source / "GPT_SoVITS/pretrained_models/sv/pretrained_eres2netv2w24s4ep4.ckpt"
    if require_reference:
        required["参考音频（请填写 ref_audio_path，不能用其他角色代替）"] = source_path(profile, "ref_audio_path")
    return required


def required_resources(profile, require_reference=True):
    _, selected = select_voice(profile)
    resources = single_voice_resources(selected, require_reference)
    if profile.get("voices") is not None:
        for voice_id, voice in voice_catalog(profile).items():
            prefix = "音色[" + voice_id + "] " + voice.get("name", voice_id) + " "
            resources[prefix + "GPT 权重"] = source_path(voice, "gpt_weights")
            resources[prefix + "SoVITS 权重"] = source_path(voice, "sovits_weights")
            if require_reference:
                resources[prefix + "参考音频"] = source_path(voice, "ref_audio_path")
    return resources


def validate_files(profile, require_reference=True):
    for label, path in required_resources(profile, require_reference).items():
        if path is None or not path.is_file() or path.stat().st_size == 0:
            raise ValueError("缺少" + label + ("：" + str(path) if path else ""))
        if not within(path):
            raise ValueError(label + " 必须位于 whale-demo 文件夹内")
    if require_reference:
        for voice in voice_catalog(profile).values():
            if not isinstance(voice.get("prompt_text", ""), str):
                raise ValueError("prompt_text 必须是参考音频的实际文字")


def validate_interpreter(profile):
    expected = source_path(profile, "python")
    if Path(sys.executable).resolve() != expected:
        raise ValueError("请使用 whale-demo 自带 Python 启动语音服务：" + str(expected))
    for path in sys.path:
        if path and not within(path):
            raise ValueError("Python 导入路径仍依赖 whale-demo 外部资源：" + str(path))


def configure_runtime():
    runtime = APP_ROOT / "data" / "sovits-runtime"
    runtime.mkdir(parents=True, exist_ok=True)
    locations = {
        "TEMP": "tmp", "TMP": "tmp", "TMPDIR": "tmp",
        "HF_HOME": "huggingface", "HF_HUB_CACHE": "huggingface/hub",
        "TRANSFORMERS_CACHE": "huggingface/transformers", "TORCH_HOME": "torch",
        "TORCH_EXTENSIONS_DIR": "torch-extensions", "NUMBA_CACHE_DIR": "numba",
        "MPLCONFIGDIR": "matplotlib", "XDG_CACHE_HOME": "cache",
        "TRITON_CACHE_DIR": "triton", "CUDA_CACHE_PATH": "cuda",
    }
    for key, relative in locations.items():
        directory = runtime / relative
        directory.mkdir(parents=True, exist_ok=True)
        os.environ[key] = str(directory)
    os.environ["PYTHONDONTWRITEBYTECODE"] = "1"
    os.environ["PYTHONIOENCODING"] = "utf-8"
    # All required models must already exist; do not download into the source.
    os.environ["HF_HUB_OFFLINE"] = "1"
    os.environ["TRANSFORMERS_OFFLINE"] = "1"
    sys.dont_write_bytecode = True
    tempfile.tempdir = str(runtime / "tmp")
    return runtime


def write_guard(event, args):
    """Reject Python filesystem mutations outside whale, including caches.

    This is a guard for trusted local inference code, not a sandbox for untrusted
    Python/native extensions. Native cache locations are redirected separately.
    """
    paths = ()
    if event == "open":
        path, mode, flags = args
        # Libraries such as dill probe file types using the operating system's
        # null device. This discards bytes and cannot modify a filesystem file.
        if not isinstance(path, int) and os.path.normcase(os.fsdecode(path)) == os.path.normcase(os.devnull):
            return
        writing = isinstance(mode, str) and any(char in mode for char in "wax+")
        writing = writing or isinstance(flags, int) and bool(flags & (os.O_WRONLY | os.O_RDWR | os.O_CREAT | os.O_TRUNC | os.O_APPEND))
        if writing and not isinstance(path, int):
            paths = (path,)
    elif event == "os.mkdir":
        # makedirs(exist_ok=True) audits even a directory that already exists.
        if not Path(args[0]).is_dir():
            paths = (args[0],)
    elif event in {"os.remove", "os.rmdir", "os.chmod", "os.chown", "os.utime", "os.truncate"}:
        paths = (args[0],)
    elif event in {"os.rename", "os.link"}:
        paths = args[:2]
    elif event == "os.symlink":
        paths = (args[1],)
    for path in paths:
        if not within(os.fsdecode(path)):
            raise PermissionError("推理写入已拦截，所有修改必须位于 whale 内：" + os.fsdecode(path))


def load_pipeline(profile, runtime):
    # Also support direct callers with a complete catalog, not just main().
    _, profile = select_voice(profile)
    validate_interpreter(profile)
    source = Path(profile["source_dir"]).resolve()
    os.chdir(source)  # Upstream resolves G2PW, SV and vocoders relative to cwd.
    sys.path[:0] = [str(source), str(source / "GPT_SoVITS")]
    os.environ["PATH"] = os.pathsep.join([str(source), str(source / "runtime"), os.environ.get("PATH", "")])
    os.environ["bert_path"] = str(source / "GPT_SoVITS/pretrained_models/chinese-roberta-wwm-ext-large")
    os.environ["NLTK_DATA"] = str(source / "runtime/nltk_data")
    sys.addaudithook(write_guard)

    # pyopenjtalk writes user dictionaries through native code, which does not
    # produce Python open audit events. Never let it regenerate the source file.
    import pyopenjtalk
    original_index = pyopenjtalk.mecab_dict_index

    def safe_dictionary_index(input_path, output_path, *args, **kwargs):
        if not within(output_path):
            raise PermissionError("不允许在 GPT-SoVITS 源目录重新生成日语词典")
        return original_index(input_path, output_path, *args, **kwargs)

    pyopenjtalk.mecab_dict_index = safe_dictionary_index
    import yaml
    from GPT_SoVITS.TTS_infer_pack.TTS import TTS, TTS_Config
    config_path = runtime / "tts_infer.yaml"
    custom = {
        "device": profile["device"], "is_half": profile["is_half"], "version": profile["version"],
        "t2s_weights_path": str(source_path(profile, "gpt_weights")),
        "vits_weights_path": str(source_path(profile, "sovits_weights")),
        "bert_base_path": str(source / "GPT_SoVITS/pretrained_models/chinese-roberta-wwm-ext-large"),
        "cnhuhbert_base_path": str(source / "GPT_SoVITS/pretrained_models/chinese-hubert-base"),
    }
    with config_path.open("w", encoding="utf-8") as stream:
        yaml.safe_dump({"custom": custom}, stream, allow_unicode=True)
    config = TTS_Config(str(config_path))
    print(config, flush=True)
    return TTS(config)


def synthesis_request(profile, payload):
    voice_id, profile = select_voice(profile, payload.get("voice"))
    if any(key in payload for key in ("gpt_weights", "sovits_weights", "weights_path", "source_dir", "python", "version")):
        raise ValueError("模型权重由已配置音色选择，不能通过请求指定路径")
    text = payload.get("text")
    if not isinstance(text, str) or not text.strip() or len(text) > 4000:
        raise ValueError("text 需为 1 到 4000 字的文字")
    if payload.get("streaming_mode", False) or payload.get("media_type", "wav") != "wav":
        raise ValueError("鲸伴本地推理仅支持非流式 WAV")
    request = {
        "text": text, "text_lang": profile["text_lang"], "voice": voice_id,
        "ref_audio_path": str(source_path(profile, "ref_audio_path") or ""),
        "prompt_text": profile.get("prompt_text", ""), "prompt_lang": profile["prompt_lang"],
        "text_split_method": profile.get("text_split_method", "cut5"),
        "speed_factor": profile.get("speed_factor", 1.0),
        "top_k": 15, "top_p": 1.0, "temperature": 1.0, "batch_size": 1,
        "split_bucket": True, "seed": -1, "parallel_infer": True,
        "repetition_penalty": 1.35, "sample_steps": 32, "super_sampling": False,
        "streaming_mode": False, "return_fragment": False, "fixed_length_chunk": False,
        "aux_ref_audio_paths": [],
    }
    bound_keys = ("ref_audio_path", "prompt_text", "prompt_lang")
    if profile.get("_catalog_voice"):
        for key in bound_keys:
            if key not in payload:
                continue
            value = payload[key]
            expected = request[key]
            if key == "ref_audio_path":
                value = str(demo_path(value, label=key))
            if value != expected:
                raise ValueError("参考音频和原文必须与当前音色配置一致")
    override_keys = ("text_lang", "text_split_method", "speed_factor")
    if not profile.get("_catalog_voice"):
        override_keys += bound_keys
    for key in override_keys:
        if key in payload:
            request[key] = payload[key]
    if request["text_lang"] not in LANGUAGES or request["prompt_lang"] not in LANGUAGES:
        raise ValueError("不支持的语音语言代码")
    if profile["version"] == "v1" and request["text_lang"] in {"yue", "all_yue", "auto_yue"}:
        raise ValueError("当前模型版本不支持粤语，请使用 v2 或更新的模型")
    speed = request["speed_factor"]
    if isinstance(speed, bool) or not isinstance(speed, (int, float)) or not 0.5 <= speed <= 2.0:
        raise ValueError("speed_factor 应在 0.5 到 2.0 之间")
    if not request["ref_audio_path"]:
        raise ValueError("请在 config/sovits.json 填写实际可用的爱音 ref_audio_path")
    reference = demo_path(request["ref_audio_path"], label="ref_audio_path")
    if not reference.is_file():
        raise ValueError("请在 config/sovits.json 填写实际可用的爱音 ref_audio_path")
    request["ref_audio_path"] = str(reference)
    return request


class PipelineVoiceManager:
    """One resident pipeline; callers must hold the application inference lock."""

    def __init__(self, profile, pipeline, loader=None):
        self.profile = profile
        self.pipeline = pipeline
        self.loader = loader
        self.active_voice, self.active_profile = select_voice(profile)

    @staticmethod
    def clear_reference_cache(pipeline):
        pipeline.prompt_cache = {
            "ref_audio_path": None, "prompt_semantic": None, "refer_spec": [],
            "prompt_text": None, "prompt_lang": None, "phones": None,
            "bert_features": None, "norm_text": None, "aux_ref_audio_paths": [],
            "raw_audio": None, "raw_sr": None,
        }
        pipeline.stop_flag = False

    @staticmethod
    def release_models(pipeline, all_models=False):
        if pipeline is None:
            return
        if all_models:
            PipelineVoiceManager.clear_reference_cache(pipeline)
        fields = ["t2s_model", "vits_model"]
        if all_models:
            fields += ["bert_model", "cnhuhbert_model", "sv_model", "vocoder", "sr_model", "text_preprocessor"]
        for field in fields:
            if hasattr(pipeline, field):
                setattr(pipeline, field, None)
        if hasattr(pipeline, "empty_cache"):
            pipeline.empty_cache()

    def invalidate(self):
        previous = self.pipeline
        self.pipeline = None
        self.active_voice = None
        self.active_profile = None
        self.release_models(previous, all_models=True)

    def select(self, voice_id):
        voice_id, target = select_voice(self.profile, voice_id)
        if self.pipeline is not None and voice_id == self.active_voice:
            return self.pipeline
        previous = self.pipeline
        previous_profile = self.active_profile
        # No later request may use a partially switched pair of checkpoints.
        self.pipeline = None
        self.active_voice = None
        self.active_profile = None
        try:
            can_switch = (previous is not None and previous_profile is not None
                          and all(hasattr(previous, method) for method in ("init_vits_weights", "init_t2s_weights"))
                          and all(previous_profile.get(key) == target.get(key) for key in ("version", "device", "is_half", "source_dir")))
            if can_switch:
                self.release_models(previous)
                # VITS establishes the actual version before the semantic
                # model calculates its version-specific mute embedding.
                previous.init_vits_weights(str(source_path(target, "sovits_weights")))
                previous.init_t2s_weights(str(source_path(target, "gpt_weights")))
                self.clear_reference_cache(previous)
                loaded = previous
            else:
                self.release_models(previous, all_models=True)
                if self.loader is None:
                    raise RuntimeError("切换音色需要重新加载本地模型")
                loaded = self.loader(target)
                self.clear_reference_cache(loaded)
            self.pipeline = loaded
            self.active_voice = voice_id
            self.active_profile = target
            return loaded
        except Exception:
            self.release_models(previous, all_models=True)
            raise


def create_app(profile, pipeline, loader=None):
    from fastapi import FastAPI
    from fastapi.responses import JSONResponse, Response
    import soundfile

    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
    inference_lock = threading.Lock()
    manager = PipelineVoiceManager(profile, pipeline, loader)
    app.state.voice_manager = manager

    @app.get("/health")
    def health():
        _, default = select_voice(profile)
        version = (manager.active_profile or default)["version"]
        return {"ok": True, "app": "whale-sovits", "version": version, "busy": inference_lock.locked(),
                "active_voice": manager.active_voice, "default_voice": default_voice_id(profile),
                "voices": public_voices(profile),
                "speech_languages": ["zh"] if version == "v1" else ["zh", "yue"]}

    @app.post("/tts")
    def tts(payload: dict):
        try:
            request = synthesis_request(profile, payload)
        except (ValueError, TypeError) as error:
            return JSONResponse(status_code=400, content={"message": str(error)})
        if not inference_lock.acquire(blocking=False):
            return JSONResponse(status_code=503, content={"message": "正在合成上一段语音，请稍后重试"})
        try:
            selected_pipeline = manager.select(request["voice"])
            generator = selected_pipeline.run(request)
            result = None
            try:
                # Upstream yields a silent placeholder before re-raising some
                # failures. Exhaust it so those errors and cleanup are observed.
                for item in generator:
                    if result is not None:
                        raise RuntimeError("非流式推理意外返回了多段音频")
                    result = item
            finally:
                generator.close()
            if result is None:
                raise RuntimeError("GPT-SoVITS 没有返回音频")
            rate, samples = result
            buffer = io.BytesIO()
            soundfile.write(buffer, samples, rate, format="WAV")
            return Response(buffer.getvalue(), media_type="audio/wav")
        except Exception as error:
            traceback.print_exc()
            manager.invalidate()
            return JSONResponse(status_code=500, content={"message": "GPT-SoVITS 推理失败，请查看 data/sovits-error.log", "exception": str(error)})
        finally:
            inference_lock.release()

    return app


def main():
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description="Whale local GPT-SoVITS bridge (read-only source)")
    parser.add_argument("--profile", default=str(APP_ROOT / "config/sovits.json"))
    parser.add_argument("--port", type=int, default=9880)
    args = parser.parse_args()
    try:
        profile = read_profile(args.profile)
        runtime = configure_runtime()
        validate_files(profile)
        validate_interpreter(profile)
        missing = [name for name in ("torch", "torchaudio", "fastapi", "uvicorn", "soundfile", "yaml", "transformers", "pyopenjtalk") if importlib.util.find_spec(name) is None]
        if missing:
            raise ValueError("当前 Python 缺少依赖，请用 GPT-SoVITS 自带 runtime/python.exe：" + ", ".join(missing))
        source = Path(profile["source_dir"])
        ffmpeg_path = str(source / "runtime")
        if not shutil.which("ffmpeg", path=ffmpeg_path):
            raise ValueError("未找到 ffmpeg，请检查 GPT-SoVITS 自带运行环境")
        _, initial_profile = select_voice(profile)
        pipeline = load_pipeline(initial_profile, runtime)
        app = create_app(profile, pipeline, loader=lambda selected: load_pipeline(selected, runtime))
        import uvicorn
        uvicorn.run(app, host="127.0.0.1", port=args.port, workers=1)
        return 0
    except Exception as error:
        print(str(error), file=sys.stderr, flush=True)
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
