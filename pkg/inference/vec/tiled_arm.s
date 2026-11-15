//go:build arm && !noasm

#include "textflag.h"

TEXT ·mulAddTiledNEON(SB), NOSPLIT, $0-20
	MOVW	dst+0(FP), R0
	MOVW	src+4(FP), R1
	MOVW	w+8(FP), R2
	MOVW	rows+12(FP), R5
	MOVW	c+16(FP), R3

row:
	MOVW	R2, R7
	MOVW	R3, R6

four:
	WORD	$0xF4210A8D // VLD1.32 {d0-d1}, [r1]!
	WORD	$0xF4272A8D // VLD1.32 {d2-d3}, [r7]!
	WORD	$0xF4204A8F // VLD1.32 {d4-d5}, [r0]
	WORD	$0xF2004D52 // VMLA.F32 q2, q0, q1
	WORD	$0xF4004A8D // VST1.32 {d4-d5}, [r0]!
	SUB.S	$4, R6
	BNE	four

	SUB.S	$1, R5
	BNE	row
	RET
