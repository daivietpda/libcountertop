//go:build arm && !noasm

#include "textflag.h"

TEXT ·addNEON(SB), NOSPLIT, $0-16
	MOVW	dst+0(FP), R0
	MOVW	a+4(FP), R1
	MOVW	b+8(FP), R2
	MOVW	n+12(FP), R3

addeight:
	CMP	$8, R3
	BLT	addone

	WORD	$0xF461028D // VLD1.32 {d16-d19}, [r1]!
	WORD	$0xF462428D // VLD1.32 {d20-d23}, [r2]!
	WORD	$0xF2400DE4 // VADD.F32 q8, q8, q10
	WORD	$0xF2422DE6 // VADD.F32 q9, q9, q11
	WORD	$0xF440028D // VST1.32 {d16-d19}, [r0]!
	SUB	$8, R3
	B	addeight

addone:
	CMP	$0, R3
	BEQ	adddone

	MOVF	0(R1), F4
	MOVF	0(R2), F5
	ADDF	F5, F4
	MOVF	F4, 0(R0)
	ADD	$4, R0
	ADD	$4, R1
	ADD	$4, R2
	SUB	$1, R3
	B	addone

adddone:
	RET

TEXT ·mulNEON(SB), NOSPLIT, $0-16
	MOVW	dst+0(FP), R0
	MOVW	a+4(FP), R1
	MOVW	b+8(FP), R2
	MOVW	n+12(FP), R3

muleight:
	CMP	$8, R3
	BLT	mulone

	WORD	$0xF461028D // VLD1.32 {d16-d19}, [r1]!
	WORD	$0xF462428D // VLD1.32 {d20-d23}, [r2]!
	WORD	$0xF3400DF4 // VMUL.F32 q8, q8, q10
	WORD	$0xF3422DF6 // VMUL.F32 q9, q9, q11
	WORD	$0xF440028D // VST1.32 {d16-d19}, [r0]!
	SUB	$8, R3
	B	muleight

mulone:
	CMP	$0, R3
	BEQ	muldone

	MOVF	0(R1), F4
	MOVF	0(R2), F5
	MULF	F5, F4
	MOVF	F4, 0(R0)
	ADD	$4, R0
	ADD	$4, R1
	ADD	$4, R2
	SUB	$1, R3
	B	mulone

muldone:
	RET
