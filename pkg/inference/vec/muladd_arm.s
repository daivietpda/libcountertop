//go:build arm && !noasm

#include "textflag.h"

TEXT ·mulAddNEON(SB), NOSPLIT, $0-16
	MOVW	dst+0(FP), R0
	MOVW	a+4(FP), R1
	MOVW	b+8(FP), R2
	MOVW	n+12(FP), R3

eight:
	CMP	$8, R3
	BLT	one

	WORD	$0xF460028F // VLD1.32 {d16-d19}, [r0]
	WORD	$0xF461428D // VLD1.32 {d20-d23}, [r1]!
	WORD	$0xF462828D // VLD1.32 {d24-d27}, [r2]!
	WORD	$0xF2440DF8 // VMLA.F32 q8, q10, q12
	WORD	$0xF2462DFA // VMLA.F32 q9, q11, q13
	WORD	$0xF440028D // VST1.32 {d16-d19}, [r0]!
	SUB	$8, R3
	B	eight

one:
	CMP	$0, R3
	BEQ	done

	MOVF	0(R0), F4
	MOVF	0(R1), F5
	MOVF	0(R2), F6
	MULAF	F6, F5, F4
	MOVF	F4, 0(R0)
	ADD	$4, R0
	ADD	$4, R1
	ADD	$4, R2
	SUB	$1, R3
	B	one

done:
	RET
