// C helper wrapping sherpa-onnx struct construction.
// Compiled into a shared library loaded by purego at runtime.
// This avoids needing purego FFI bindings for dozens of C structs.

#include "c-api.h"
#include <string.h>

static void setVits(SherpaOnnxOfflineTtsConfig* cfg,
    const char* model, const char* tokens, const char* lexicon) {
    cfg->model.vits.model = model;
    cfg->model.vits.tokens = tokens;
    cfg->model.vits.lexicon = lexicon;
    cfg->model.num_threads = 2;
}

static void setKokoro(SherpaOnnxOfflineTtsConfig* cfg,
    const char* model, const char* tokens, const char* voices,
    const char* data_dir, const char* lexicon) {
    cfg->model.kokoro.model = model;
    cfg->model.kokoro.tokens = tokens;
    cfg->model.kokoro.voices = voices;
    cfg->model.kokoro.data_dir = data_dir;
    cfg->model.kokoro.lexicon = lexicon;
    cfg->model.num_threads = 2;
}

static void setMatcha(SherpaOnnxOfflineTtsConfig* cfg,
    const char* acoustic, const char* vocoder, const char* tokens,
    const char* lexicon, const char* data_dir) {
    cfg->model.matcha.acoustic_model = acoustic;
    cfg->model.matcha.vocoder = vocoder;
    cfg->model.matcha.tokens = tokens;
    cfg->model.matcha.lexicon = lexicon;
    cfg->model.matcha.data_dir = data_dir;
    cfg->model.num_threads = 2;
}

void* tts_create_vits(const char* m, const char* t, const char* l) {
    SherpaOnnxOfflineTtsConfig c; memset(&c, 0, sizeof(c));
    setVits(&c, m, t, l);
    return (void*)SherpaOnnxCreateOfflineTts(&c);
}

void* tts_create_kokoro(const char* m, const char* t, const char* v,
                        const char* d, const char* l) {
    SherpaOnnxOfflineTtsConfig c; memset(&c, 0, sizeof(c));
    setKokoro(&c, m, t, v, d, l);
    return (void*)SherpaOnnxCreateOfflineTts(&c);
}

void* tts_create_matcha(const char* a, const char* v, const char* t,
                        const char* l, const char* d) {
    SherpaOnnxOfflineTtsConfig c; memset(&c, 0, sizeof(c));
    setMatcha(&c, a, v, t, l, d);
    return (void*)SherpaOnnxCreateOfflineTts(&c);
}

void  tts_destroy(void* p)         { SherpaOnnxDestroyOfflineTts(p); }
void* tts_generate(void* p, const char* t, int s, float sp) {
    SherpaOnnxGenerationConfig g;
    memset(&g, 0, sizeof(g));
    g.sid = s;
    g.speed = sp;
    return (void*)SherpaOnnxOfflineTtsGenerateWithConfig(
        (const SherpaOnnxOfflineTts*)p, t, &g, NULL, NULL);
}
int   tts_num_speakers(void* p)    { return SherpaOnnxOfflineTtsNumSpeakers(p); }
void  tts_free_audio(void* p)      { SherpaOnnxDestroyOfflineTtsGeneratedAudio(p); }
const float* tts_audio_samps(void* p) { return ((const SherpaOnnxGeneratedAudio*)p)->samples; }
int   tts_audio_n(void* p)         { return ((const SherpaOnnxGeneratedAudio*)p)->n; }
int   tts_audio_sr(void* p)        { return ((const SherpaOnnxGeneratedAudio*)p)->sample_rate; }

void* asr_create(const char* enc, const char* dec, const char* toks,
                 const char* model, int is_whisper) {
    SherpaOnnxOfflineRecognizerConfig c; memset(&c, 0, sizeof(c));
    c.feat_config.sample_rate = 16000;
    c.feat_config.feature_dim = 80;
    c.model_config.tokens = toks;
    c.model_config.num_threads = 2;
    if (is_whisper) {
        c.model_config.whisper.encoder = enc;
        c.model_config.whisper.decoder = dec;
    } else {
        c.model_config.sense_voice.model = model;
    }
    return (void*)SherpaOnnxCreateOfflineRecognizer(&c);
}

void  asr_destroy(void* p)       { SherpaOnnxDestroyOfflineRecognizer(p); }
void* asr_create_stream(void* p) { return (void*)SherpaOnnxCreateOfflineStream(p); }
void  asr_destroy_stream(void* s){ SherpaOnnxDestroyOfflineStream(s); }
void  asr_accept_waveform(void* s, int sr, const float* samps, int n) {
    SherpaOnnxAcceptWaveformOffline(s, sr, samps, n);
}
void  asr_decode(void* r, void* s) { SherpaOnnxDecodeOfflineStream(r, s); }
const char* asr_get_text(void* s) {
    const SherpaOnnxOfflineRecognizerResult* r = SherpaOnnxGetOfflineStreamResult(s);
    return r ? r->text : NULL;
}

void* wave_read(const char* p)   { return (void*)SherpaOnnxReadWave(p); }
void  wave_destroy(void* w)      { SherpaOnnxFreeWave(w); }
const float* wave_samps(void* w) { return ((const SherpaOnnxWave*)w)->samples; }
int   wave_sr(void* w)           { return ((const SherpaOnnxWave*)w)->sample_rate; }
int   wave_n(void* w)            { return ((const SherpaOnnxWave*)w)->num_samples; }
