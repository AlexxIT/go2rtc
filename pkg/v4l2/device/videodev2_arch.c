//go:build ignore
#include <stdio.h>
#include <stddef.h>
#include <linux/videodev2.h>

#define printconst1(con) printf("\t%s = 0x%08lx\n", #con, con)
#define printconst2(con) printf("\t%s = %d\n", #con, con)
#define printstruct(str) printf("type %s struct { // size %lu\n", #str, sizeof(struct str))
#define printmember(str, mem, typ) printf("\t%s %s // offset %lu, size %lu\n", #mem == "type" ? "typ" : #mem, typ, offsetof(struct str, mem), sizeof((struct str){0}.mem))
#define printunimem(str, uni, mem, typ) printf("\t%s %s // offset %lu, size %lu\n", #mem, typ, offsetof(struct str, uni.mem), sizeof((struct str){0}.uni.mem))
#define printalign1(str, mem2, mem1) printf("\t_ [%lu]byte // align\n", offsetof(struct str, mem2) - offsetof(struct str, mem1) - sizeof((struct str){0}.mem1))
#define printfiller(str, mem) printf("\t_ [%lu]byte // filler\n", sizeof(struct str) - offsetof(struct str, mem) - sizeof((struct str){0}.mem))

int main() {
	printf("const (\n");
	printconst1(VIDIOC_QUERYCAP);
	printconst1(VIDIOC_ENUM_FMT);
	printconst1(VIDIOC_G_FMT);
	printconst1(VIDIOC_S_FMT);
	printconst1(VIDIOC_REQBUFS);
	printconst1(VIDIOC_QUERYBUF);
	printf("\n");
	printconst1(VIDIOC_QBUF);
	printconst1(VIDIOC_DQBUF);
	printconst1(VIDIOC_STREAMON);
	printconst1(VIDIOC_STREAMOFF);
	printconst1(VIDIOC_G_PARM);
	printconst1(VIDIOC_S_PARM);
	printf("\n");
	printconst1(VIDIOC_ENUM_FRAMESIZES);
	printconst1(VIDIOC_ENUM_FRAMEINTERVALS);
	printf(")\n\n");

	printf("const (\n");
	printconst2(V4L2_BUF_TYPE_VIDEO_CAPTURE);
	printconst2(V4L2_COLORSPACE_DEFAULT);
	printconst2(V4L2_FIELD_NONE);
	printconst2(V4L2_FRMIVAL_TYPE_DISCRETE);
	printconst2(V4L2_FRMSIZE_TYPE_DISCRETE);
	printconst2(V4L2_MEMORY_MMAP);
	printf(")\n\n");

	printstruct(v4l2_capability);
	printmember(v4l2_capability, driver, "[16]byte");
	printmember(v4l2_capability, card, "[32]byte");
	printmember(v4l2_capability, bus_info, "[32]byte");
	printmember(v4l2_capability, version, "uint32");
	printmember(v4l2_capability, capabilities, "uint32");
	printmember(v4l2_capability, device_caps, "uint32");
	printmember(v4l2_capability, reserved, "[3]uint32");
	printf("}\n\n");

	printstruct(v4l2_format);
	printmember(v4l2_format, type, "uint32");
	printalign1(v4l2_format, fmt, type);
	printunimem(v4l2_format, fmt, pix, "v4l2_pix_format");
	printfiller(v4l2_format, fmt.pix);
	printf("}\n\n");

	printstruct(v4l2_pix_format);
	printmember(v4l2_pix_format, width, "uint32");
	printmember(v4l2_pix_format, height, "uint32");
	printmember(v4l2_pix_format, pixelformat, "uint32");
	printmember(v4l2_pix_format, field, "uint32");
	printmember(v4l2_pix_format, bytesperline, "uint32");
	printmember(v4l2_pix_format, sizeimage, "uint32");
	printmember(v4l2_pix_format, colorspace, "uint32");
	printmember(v4l2_pix_format, priv, "uint32");
	printmember(v4l2_pix_format, flags, "uint32");
	printmember(v4l2_pix_format, ycbcr_enc, "uint32");
	printmember(v4l2_pix_format, quantization, "uint32");
	printmember(v4l2_pix_format, xfer_func, "uint32");
	printf("}\n\n");

	printstruct(v4l2_streamparm);
	printmember(v4l2_streamparm, type, "uint32");
	printunimem(v4l2_streamparm, parm, capture, "v4l2_captureparm");
	printfiller(v4l2_streamparm, parm.capture);
	printf("}\n\n");

	printstruct(v4l2_captureparm);
	printmember(v4l2_captureparm, capability, "uint32");
	printmember(v4l2_captureparm, capturemode, "uint32");
	printmember(v4l2_captureparm, timeperframe, "v4l2_fract");
	printmember(v4l2_captureparm, extendedmode, "uint32");
	printmember(v4l2_captureparm, readbuffers, "uint32");
	printmember(v4l2_captureparm, reserved, "[4]uint32");
	printf("}\n\n");

	printstruct(v4l2_fract);
	printmember(v4l2_fract, numerator, "uint32");
	printmember(v4l2_fract, denominator, "uint32");
	printf("}\n\n");

	printstruct(v4l2_requestbuffers);
	printmember(v4l2_requestbuffers, count, "uint32");
	printmember(v4l2_requestbuffers, type, "uint32");
	printmember(v4l2_requestbuffers, memory, "uint32");
	printmember(v4l2_requestbuffers, capabilities, "uint32");
	printmember(v4l2_requestbuffers, flags, "uint8");
	printmember(v4l2_requestbuffers, reserved, "[3]uint8");
	printf("}\n\n");

	printstruct(v4l2_buffer);
	printmember(v4l2_buffer, index, "uint32");
	printmember(v4l2_buffer, type, "uint32");
	printmember(v4l2_buffer, bytesused, "uint32");
	printmember(v4l2_buffer, flags, "uint32");
	printmember(v4l2_buffer, field, "uint32");
	printalign1(v4l2_buffer, timecode, field);
	printmember(v4l2_buffer, timecode, "v4l2_timecode");
	printmember(v4l2_buffer, sequence, "uint32");
	printmember(v4l2_buffer, memory, "uint32");
	printunimem(v4l2_buffer, m, offset, "uint32");
	printalign1(v4l2_buffer, length, m.offset);
	printmember(v4l2_buffer, length, "uint32");
	printfiller(v4l2_buffer, length);
	printf("}\n\n");

	printstruct(v4l2_timecode);
	printmember(v4l2_timecode, type, "uint32");
	printmember(v4l2_timecode, flags, "uint32");
	printmember(v4l2_timecode, frames, "uint8");
	printmember(v4l2_timecode, seconds, "uint8");
	printmember(v4l2_timecode, minutes, "uint8");
	printmember(v4l2_timecode, hours, "uint8");
	printmember(v4l2_timecode, userbits, "[4]uint8");
	printf("}\n\n");

	printstruct(v4l2_fmtdesc);
	printmember(v4l2_fmtdesc, index, "uint32");
	printmember(v4l2_fmtdesc, type, "uint32");
	printmember(v4l2_fmtdesc, flags, "uint32");
	printmember(v4l2_fmtdesc, description, "[32]byte");
	printmember(v4l2_fmtdesc, pixelformat, "uint32");
	printmember(v4l2_fmtdesc, mbus_code, "uint32");
	printmember(v4l2_fmtdesc, reserved, "[3]uint32");
	printf("}\n\n");

	printstruct(v4l2_frmsizeenum);
	printmember(v4l2_frmsizeenum, index, "uint32");
	printmember(v4l2_frmsizeenum, pixel_format, "uint32");
	printmember(v4l2_frmsizeenum, type, "uint32");
	printmember(v4l2_frmsizeenum, discrete, "v4l2_frmsize_discrete");
	printfiller(v4l2_frmsizeenum, discrete);
	printf("}\n\n");

	printstruct(v4l2_frmsize_discrete);
	printmember(v4l2_frmsize_discrete, width, "uint32");
	printmember(v4l2_frmsize_discrete, height, "uint32");
	printf("}\n\n");

	printstruct(v4l2_frmivalenum);
	printmember(v4l2_frmivalenum, index, "uint32");
	printmember(v4l2_frmivalenum, pixel_format, "uint32");
	printmember(v4l2_frmivalenum, width, "uint32");
	printmember(v4l2_frmivalenum, height, "uint32");
	printmember(v4l2_frmivalenum, type, "uint32");
	printmember(v4l2_frmivalenum, discrete, "v4l2_fract");
	printfiller(v4l2_frmivalenum, discrete);
	printf("}\n\n");

	return 0;
}