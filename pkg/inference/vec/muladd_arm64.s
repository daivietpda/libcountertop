//go:build arm64 && !noasm

#include "textflag.h"

TEXT ·mulAddNEON(SB), NOSPLIT, $0-32
	MOVD	dst+0(FP), R0
	MOVD	a+8(FP), R1
	MOVD	b+16(FP), R2
	MOVD	n+24(FP), R3

eight:
	CMP	$8, R3
	BLT	one

	VLD1.P	32(R1), [V4.S4, V5.S4]
	VLD1.P	32(R2), [V6.S4, V7.S4]
	VLD1	(R0), [V8.S4, V9.S4]
	VFMLA	V4.S4, V6.S4, V8.S4
	VFMLA	V5.S4, V7.S4, V9.S4
	VST1.P	[V8.S4, V9.S4], 32(R0)
	SUB	$8, R3
	B	eight

one:
	CBZ	R3, done

single:
	FMOVS	(R0), F0
	FMOVS	(R1), F1
	FMOVS	(R2), F2
	FMADDS	F2, F0, F1, F0
	FMOVS	F0, (R0)
	ADD	$4, R0
	ADD	$4, R1
	ADD	$4, R2
	SUB	$1, R3
	CBNZ	R3, single

done:
	RET
