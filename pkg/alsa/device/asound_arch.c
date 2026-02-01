//go:build ignore
#include <stdio.h>
#include <stddef.h>
#include <sys/ioctl.h>
#include <sound/asound.h>

#define print_line(text) printf("%s\n", text)
#define print_hex_const(name) printf("\t%s = 0x%08lx\n", #name, name)
#define print_int_const(con) printf("\t%s = %d\n", #con, con)

#define print_struct_header(str) printf("type %s struct { // size %lu\n", #str, sizeof(struct str))
#define print_struct_member(str, mem, typ) printf("\t%s %s // offset %lu, size %lu\n", #mem == "type" ? "typ" : #mem, typ, offsetof(struct str, mem), sizeof((struct str){0}.mem))

// https://github.com/torvalds/linux/blob/master/include/uapi/sound/asound.h
int main() {
    print_line("package device\n");

    print_line("type unsigned_char = byte");
    print_line("type signed_int = int32");
    print_line("type unsigned_int = uint32");
    print_line("type signed_long = int64");
    print_line("type unsigned_long = uint64");
    print_line("type __u32 = uint32");
    print_line("type void__user = uintptr\n");

    print_line("const (");
    print_int_const(SNDRV_PCM_STREAM_PLAYBACK);
    print_int_const(SNDRV_PCM_STREAM_CAPTURE);
	print_line("");
	print_int_const(SNDRV_PCM_ACCESS_MMAP_INTERLEAVED);
	print_int_const(SNDRV_PCM_ACCESS_MMAP_NONINTERLEAVED);
	print_int_const(SNDRV_PCM_ACCESS_MMAP_COMPLEX);
	print_int_const(SNDRV_PCM_ACCESS_RW_INTERLEAVED);
	print_int_const(SNDRV_PCM_ACCESS_RW_NONINTERLEAVED);
	print_line("");
	print_int_const(SNDRV_PCM_FORMAT_S8);
	print_int_const(SNDRV_PCM_FORMAT_U8);
	print_int_const(SNDRV_PCM_FORMAT_S16_LE);
	print_int_const(SNDRV_PCM_FORMAT_S16_BE);
	print_int_const(SNDRV_PCM_FORMAT_U16_LE);
	print_int_const(SNDRV_PCM_FORMAT_U16_BE);
	print_int_const(SNDRV_PCM_FORMAT_S24_LE);
	print_int_const(SNDRV_PCM_FORMAT_S24_BE);
	print_int_const(SNDRV_PCM_FORMAT_U24_LE);
	print_int_const(SNDRV_PCM_FORMAT_U24_BE);
	print_int_const(SNDRV_PCM_FORMAT_S32_LE);
	print_int_const(SNDRV_PCM_FORMAT_S32_BE);
	print_int_const(SNDRV_PCM_FORMAT_U32_LE);
	print_int_const(SNDRV_PCM_FORMAT_U32_BE);
	print_int_const(SNDRV_PCM_FORMAT_FLOAT_LE);
	print_int_const(SNDRV_PCM_FORMAT_FLOAT_BE);
	print_int_const(SNDRV_PCM_FORMAT_FLOAT64_LE);
	print_int_const(SNDRV_PCM_FORMAT_FLOAT64_BE);
	print_int_const(SNDRV_PCM_FORMAT_MU_LAW);
	print_int_const(SNDRV_PCM_FORMAT_A_LAW);
	print_int_const(SNDRV_PCM_FORMAT_MPEG);
	print_line("");
    print_hex_const(SNDRV_PCM_IOCTL_PVERSION);        // A 0x00
    print_hex_const(SNDRV_PCM_IOCTL_INFO);            // A 0x01
    print_hex_const(SNDRV_PCM_IOCTL_HW_REFINE);       // A 0x10
    print_hex_const(SNDRV_PCM_IOCTL_HW_PARAMS);       // A 0x11
    print_hex_const(SNDRV_PCM_IOCTL_SW_PARAMS);       // A 0x13
    print_hex_const(SNDRV_PCM_IOCTL_PREPARE);         // A 0x40
    print_hex_const(SNDRV_PCM_IOCTL_WRITEI_FRAMES);   // A 0x50
    print_hex_const(SNDRV_PCM_IOCTL_READI_FRAMES);    // A 0x51
    print_line(")\n");

	print_struct_header(snd_pcm_info);
	print_struct_member(snd_pcm_info, device, "unsigned_int");
	print_struct_member(snd_pcm_info, subdevice, "unsigned_int");
	print_struct_member(snd_pcm_info, stream, "signed_int");
	print_struct_member(snd_pcm_info, card, "signed_int");
	print_struct_member(snd_pcm_info, id, "[64]unsigned_char");
	print_struct_member(snd_pcm_info, name, "[80]unsigned_char");
	print_struct_member(snd_pcm_info, subname, "[32]unsigned_char");
	print_struct_member(snd_pcm_info, dev_class, "signed_int");
	print_struct_member(snd_pcm_info, dev_subclass, "signed_int");
	print_struct_member(snd_pcm_info, subdevices_count, "unsigned_int");
	print_struct_member(snd_pcm_info, subdevices_avail, "unsigned_int");
	print_line("\tpad1 [16]unsigned_char");
	print_struct_member(snd_pcm_info, reserved, "[64]unsigned_char");
	print_line("}\n");

	print_line("type snd_pcm_uframes_t = unsigned_long");
	print_line("type snd_pcm_sframes_t = signed_long\n");

	print_struct_header(snd_xferi);
	print_struct_member(snd_xferi, result, "snd_pcm_sframes_t");
	print_struct_member(snd_xferi, buf, "void__user");
	print_struct_member(snd_xferi, frames, "snd_pcm_uframes_t");
	print_line("}\n");

	print_line("const (");
	print_int_const(SNDRV_PCM_HW_PARAM_ACCESS);
	print_int_const(SNDRV_PCM_HW_PARAM_FORMAT);
	print_int_const(SNDRV_PCM_HW_PARAM_SUBFORMAT);
	print_int_const(SNDRV_PCM_HW_PARAM_FIRST_MASK);
	print_int_const(SNDRV_PCM_HW_PARAM_LAST_MASK);
	print_line("");
	print_int_const(SNDRV_PCM_HW_PARAM_SAMPLE_BITS);
	print_int_const(SNDRV_PCM_HW_PARAM_FRAME_BITS);
	print_int_const(SNDRV_PCM_HW_PARAM_CHANNELS);
	print_int_const(SNDRV_PCM_HW_PARAM_RATE);
	print_int_const(SNDRV_PCM_HW_PARAM_PERIOD_TIME);
	print_int_const(SNDRV_PCM_HW_PARAM_PERIOD_SIZE);
	print_int_const(SNDRV_PCM_HW_PARAM_PERIOD_BYTES);
	print_int_const(SNDRV_PCM_HW_PARAM_PERIODS);
	print_int_const(SNDRV_PCM_HW_PARAM_BUFFER_TIME);
	print_int_const(SNDRV_PCM_HW_PARAM_BUFFER_SIZE);
	print_int_const(SNDRV_PCM_HW_PARAM_BUFFER_BYTES);
	print_int_const(SNDRV_PCM_HW_PARAM_TICK_TIME);
	print_int_const(SNDRV_PCM_HW_PARAM_FIRST_INTERVAL);
	print_int_const(SNDRV_PCM_HW_PARAM_LAST_INTERVAL);
	print_line("");
	print_int_const(SNDRV_MASK_MAX);
	print_line("");
	print_int_const(SNDRV_PCM_TSTAMP_NONE);
	print_int_const(SNDRV_PCM_TSTAMP_ENABLE);
	print_line(")\n");

	print_struct_header(snd_mask);
	print_struct_member(snd_mask, bits, "[(SNDRV_MASK_MAX+31)/32]__u32");
	print_line("}\n");

	print_struct_header(snd_interval);
	print_struct_member(snd_interval, min, "unsigned_int");
	print_struct_member(snd_interval, max, "unsigned_int");
	print_line("\tbit unsigned_int");
	print_line("}\n");

	print_struct_header(snd_pcm_hw_params);
	print_struct_member(snd_pcm_hw_params, flags, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, masks, "[SNDRV_PCM_HW_PARAM_LAST_MASK-SNDRV_PCM_HW_PARAM_FIRST_MASK+1]snd_mask");
	print_struct_member(snd_pcm_hw_params, mres, "[5]snd_mask");
	print_struct_member(snd_pcm_hw_params, intervals, "[SNDRV_PCM_HW_PARAM_LAST_INTERVAL-SNDRV_PCM_HW_PARAM_FIRST_INTERVAL+1]snd_interval");
	print_struct_member(snd_pcm_hw_params, ires, "[9]snd_interval");
	print_struct_member(snd_pcm_hw_params, rmask, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, cmask, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, info, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, msbits, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, rate_num, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, rate_den, "unsigned_int");
	print_struct_member(snd_pcm_hw_params, fifo_size, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_hw_params, reserved, "[64]unsigned_char");
	print_line("}\n");

	print_struct_header(snd_pcm_sw_params);
	print_struct_member(snd_pcm_sw_params, tstamp_mode, "signed_int");
	print_struct_member(snd_pcm_sw_params, period_step, "unsigned_int");
	print_struct_member(snd_pcm_sw_params, sleep_min, "unsigned_int");
	print_struct_member(snd_pcm_sw_params, avail_min, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, xfer_align, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, start_threshold, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, stop_threshold, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, silence_threshold, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, silence_size, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, boundary, "snd_pcm_uframes_t");
	print_struct_member(snd_pcm_sw_params, proto, "unsigned_int");
	print_struct_member(snd_pcm_sw_params, tstamp_type, "unsigned_int");
	print_struct_member(snd_pcm_sw_params, reserved, "[56]unsigned_char");
	print_line("}\n");

	return 0;
}