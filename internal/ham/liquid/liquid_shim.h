// liquid_shim.h — C shim between CGO and liquid-dsp.
//
// liquid-dsp's ofdmflexframe callbacks use function pointers that CGO cannot
// call directly from Go. This shim registers a single C callback that
// forwards decoded frames to Go via a CGO-exported function.
//
// Build notes:
//   CGO_LDFLAGS=-lliquid (system install)  — Linux/macOS/Pi
//   CGO_LDFLAGS=-lliquid -L/path/to/lib    — custom prefix
//   iOS: static lib compiled with Xcode toolchain, linked via CGO_LDFLAGS
//   Android: static lib compiled with NDK CMake, linked via CGO_LDFLAGS
//
// liquid-dsp source: https://github.com/jgaeddert/liquid-dsp
// Install: ./bootstrap.sh && ./configure && make && sudo make install

#ifndef LIQUID_SHIM_H
#define LIQUID_SHIM_H

#include <stdint.h>
#include <string.h>
#include <liquid/liquid.h>

// forward-declared Go callback (exported from modem.go via //export)
extern void goFrameCallback(int id, uint8_t *payload, uint32_t payloadLen);

// Per-instance shim state. Keyed by integer ID so CGO can route
// callbacks to the correct Go-side Modem struct.
typedef struct {
    int              id;
    ofdmflexframegen fg;
    ofdmflexframesync fs;
} liquid_modem_t;

// C-side callback registered with ofdmflexframesync_create.
// Forwards to Go via goFrameCallback.
static int liquid_frame_callback(
    unsigned char   *header,
    int              header_valid,
    unsigned char   *payload,
    unsigned int     payload_len,
    int              payload_valid,
    framesyncstats_s stats,
    void            *userdata)
{
    (void)header;
    (void)header_valid;
    (void)stats;
    if (!payload_valid || payload == NULL || payload_len == 0) return 0;
    liquid_modem_t *m = (liquid_modem_t *)userdata;
    goFrameCallback(m->id, (uint8_t *)payload, (uint32_t)payload_len);
    return 0;
}

// liquid_modem_create allocates and initialises an OFDM flex frame modem.
//   id:          caller-assigned integer, returned in goFrameCallback
//   subcarriers: number of OFDM subcarriers (64 recommended)
//   cp_len:      cyclic prefix length (16 recommended)
//   taper_len:   taper window length (4 recommended)
// Returns NULL on failure.
static liquid_modem_t *liquid_modem_create(int id, unsigned int subcarriers,
                                           unsigned int cp_len,
                                           unsigned int taper_len)
{
    liquid_modem_t *m = (liquid_modem_t *)malloc(sizeof(liquid_modem_t));
    if (!m) return NULL;
    m->id = id;

    // TX: default props (BPSK, NONE FEC, r½ convolutional)
    ofdmflexframegenprops_s fgprops;
    ofdmflexframegenprops_init_default(&fgprops);
    fgprops.check      = LIQUID_CRC_32;
    fgprops.fec0       = LIQUID_FEC_CONV_V27;   // rate-1/2 convolutional
    fgprops.fec1       = LIQUID_FEC_NONE;
    fgprops.mod_scheme = LIQUID_MODEM_BPSK;     // most robust

    m->fg = ofdmflexframegen_create(subcarriers, cp_len, taper_len, NULL, &fgprops);
    if (!m->fg) { free(m); return NULL; }

    // RX
    m->fs = ofdmflexframesync_create(subcarriers, cp_len, taper_len, NULL,
                                     liquid_frame_callback, m);
    if (!m->fs) { ofdmflexframegen_destroy(m->fg); free(m); return NULL; }

    return m;
}

// liquid_modem_destroy frees all resources.
static void liquid_modem_destroy(liquid_modem_t *m) {
    if (!m) return;
    ofdmflexframegen_destroy(m->fg);
    ofdmflexframesync_destroy(m->fs);
    free(m);
}

// liquid_modem_tx_frame encodes one payload into OFDM samples.
// Caller-supplied buffer must be large enough: (subcarriers+cp_len) * num_symbols * 2 floats.
// Returns the number of complex float pairs written.
// Each float pair is [real, imag] interleaved.
static unsigned int liquid_modem_tx_frame(liquid_modem_t *m,
                                          const uint8_t *payload, unsigned int payload_len,
                                          float *samples_out, unsigned int max_samples)
{
    unsigned char header[8] = {0}; // unused header, zero-filled
    ofdmflexframegen_assemble(m->fg, header, (unsigned char *)payload, payload_len);

    unsigned int written = 0;
    liquid_float_complex buf[256];
    int last_symbol = 0;
    while (!last_symbol) {
        last_symbol = ofdmflexframegen_write(m->fg, buf, 256);
        for (unsigned int i = 0; i < 256; i++) {
            if (written + 2 > max_samples) goto done;
            samples_out[written++] = crealf(buf[i]);
            samples_out[written++] = cimagf(buf[i]);
        }
    }
done:
    return written / 2; // number of complex samples
}

// liquid_modem_rx_push feeds received samples to the synchroniser.
// samples: interleaved [real, imag] float pairs, num_samples complex samples.
static void liquid_modem_rx_push(liquid_modem_t *m,
                                 const float *samples, unsigned int num_samples)
{
    // liquid-dsp expects liquid_float_complex array
    liquid_float_complex buf[256];
    while (num_samples > 0) {
        unsigned int n = num_samples < 256 ? num_samples : 256;
        for (unsigned int i = 0; i < n; i++) {
            buf[i] = samples[2*i] + samples[2*i+1] * I;
        }
        ofdmflexframesync_execute(m->fs, buf, n);
        samples    += 2 * n;
        num_samples -= n;
    }
}

#endif // LIQUID_SHIM_H
