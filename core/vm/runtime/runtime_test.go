// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package runtime

import (
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"evm/common"
	"evm/core/asm"
	"evm/core/rawdb"
	"evm/core/state"
	"evm/core/types"
	"evm/core/vm"
	"evm/params"
)

func TestDefaults(t *testing.T) {
	cfg := new(Config)
	setDefaults(cfg)

	if cfg.Difficulty == nil {
		t.Error("expected difficulty to be non nil")
	}

	if cfg.Time == nil {
		t.Error("expected time to be non nil")
	}
	if cfg.GasLimit == 0 {
		t.Error("didn't expect gaslimit to be zero")
	}
	if cfg.GasPrice == nil {
		t.Error("expected time to be non nil")
	}
	if cfg.Value == nil {
		t.Error("expected time to be non nil")
	}
	if cfg.GetHashFn == nil {
		t.Error("expected time to be non nil")
	}
	if cfg.BlockNumber == nil {
		t.Error("expected block number to be non nil")
	}
}

func TestEVM(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("crashed with: %v", r)
		}
	}()

	Execute([]byte{
		byte(vm.DIFFICULTY),
		byte(vm.TIMESTAMP),
		byte(vm.GASLIMIT),
		byte(vm.PUSH1),
		byte(vm.ORIGIN),
		byte(vm.BLOCKHASH),
		byte(vm.COINBASE),
	}, nil, nil)
}

func TestExecute(t *testing.T) {
	ret, _, err := Execute([]byte{
		byte(vm.PUSH1), 10,
		byte(vm.PUSH1), 0,
		byte(vm.MSTORE),
		byte(vm.PUSH1), 32,
		byte(vm.PUSH1), 0,
		byte(vm.RETURN),
	}, nil, nil)
	if err != nil {
		t.Fatal("didn't expect error", err)
	}

	num := new(big.Int).SetBytes(ret)
	if num.Cmp(big.NewInt(10)) != 0 {
		t.Error("Expected 10, got", num)
	}
}

func TestCall(t *testing.T) {
	state, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	address := common.HexToAddress("0x0a")
	state.SetCode(address, []byte{
		byte(vm.PUSH1), 10,
		byte(vm.PUSH1), 0,
		byte(vm.MSTORE),
		byte(vm.PUSH1), 32,
		byte(vm.PUSH1), 0,
		byte(vm.RETURN),
	})

	ret, _, err := Call(address, nil, &Config{State: state})
	if err != nil {
		t.Fatal("didn't expect error", err)
	}

	num := new(big.Int).SetBytes(ret)
	if num.Cmp(big.NewInt(10)) != 0 {
		t.Error("Expected 10, got", num)
	}
}

func benchmarkEVM_Create(bench *testing.B, code string) {
	var (
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
		sender     = common.BytesToAddress([]byte("sender"))
		receiver   = common.BytesToAddress([]byte("receiver"))
	)

	statedb.CreateAccount(sender)
	statedb.SetCode(receiver, common.FromHex(code))
	runtimeConfig := Config{
		Origin:      sender,
		State:       statedb,
		GasLimit:    10000000,
		Difficulty:  big.NewInt(0x200000),
		Time:        new(big.Int).SetUint64(0),
		Coinbase:    common.Address{},
		BlockNumber: new(big.Int).SetUint64(1),
		ChainConfig: &params.ChainConfig{
			ChainID:             big.NewInt(1),
			HomesteadBlock:      new(big.Int),
			ByzantiumBlock:      new(big.Int),
			ConstantinopleBlock: new(big.Int),
			DAOForkBlock:        new(big.Int),
			DAOForkSupport:      false,
			EIP150Block:         new(big.Int),
			EIP155Block:         new(big.Int),
			EIP158Block:         new(big.Int),
		},
		EVMConfig: vm.Config{},
	}
	// Warm up the intpools and stuff
	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		Call(receiver, []byte{}, &runtimeConfig)
	}
	bench.StopTimer()
}

func BenchmarkEVM_CREATE_500(bench *testing.B) {
	// initcode size 500K, repeatedly calls CREATE and then modifies the mem contents
	benchmarkEVM_Create(bench, "5b6207a120600080f0600152600056")
}
func BenchmarkEVM_CREATE2_500(bench *testing.B) {
	// initcode size 500K, repeatedly calls CREATE2 and then modifies the mem contents
	benchmarkEVM_Create(bench, "5b586207a120600080f5600152600056")
}
func BenchmarkEVM_CREATE_1200(bench *testing.B) {
	// initcode size 1200K, repeatedly calls CREATE and then modifies the mem contents
	benchmarkEVM_Create(bench, "5b62124f80600080f0600152600056")
}
func BenchmarkEVM_CREATE2_1200(bench *testing.B) {
	// initcode size 1200K, repeatedly calls CREATE2 and then modifies the mem contents
	benchmarkEVM_Create(bench, "5b5862124f80600080f5600152600056")
}

func fakeHeader(n uint64, parentHash common.Hash) *types.Header {
	header := types.Header{
		Coinbase:   common.HexToAddress("0x00000000000000000000000000000000deadbeef"),
		Number:     big.NewInt(int64(n)),
		ParentHash: parentHash,
		Time:       1000,
		Nonce:      types.BlockNonce{0x1},
		Extra:      []byte{},
		Difficulty: big.NewInt(0),
		GasLimit:   100000,
	}
	return &header
}

type dummyChain struct {
	counter int
}

type stepCounter struct {
	inner *vm.JSONLogger
	steps int
}

func (s *stepCounter) CaptureStart(from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) error {
	return nil
}

func (s *stepCounter) CaptureState(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, memory *vm.Memory, stack *vm.Stack, rStack *vm.ReturnStack, rData []byte, contract *vm.Contract, depth int, err error) error {
	s.steps++
	// Enable this for more output
	//s.inner.CaptureState(env, pc, op, gas, cost, memory, stack, rStack, contract, depth, err)
	return nil
}

func (s *stepCounter) CaptureFault(env *vm.EVM, pc uint64, op vm.OpCode, gas, cost uint64, memory *vm.Memory, stack *vm.Stack, rStack *vm.ReturnStack, contract *vm.Contract, depth int, err error) error {
	return nil
}

func (s *stepCounter) CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error) error {
	return nil
}

func TestJumpSub1024Limit(t *testing.T) {
	state, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	address := common.HexToAddress("0x0a")
	// Code is
	// 0 beginsub
	// 1 push 0
	// 3 jumpsub
	//
	// The code recursively calls itself. It should error when the returns-stack
	// grows above 1023
	state.SetCode(address, []byte{
		byte(vm.PUSH1), 3,
		byte(vm.JUMPSUB),
		byte(vm.BEGINSUB),
		byte(vm.PUSH1), 3,
		byte(vm.JUMPSUB),
	})
	tracer := stepCounter{inner: vm.NewJSONLogger(nil, os.Stdout)}
	// Enable 2315
	_, _, err := Call(address, nil, &Config{State: state,
		GasLimit:    20000,
		ChainConfig: params.AllEthashProtocolChanges,
		EVMConfig: vm.Config{
			ExtraEips: []int{2315},
			Debug:     true,
			//Tracer:    vm.NewJSONLogger(nil, os.Stdout),
			Tracer: &tracer,
		}})
	exp := "return stack limit reached"
	if err.Error() != exp {
		t.Fatalf("expected %v, got %v", exp, err)
	}
	if exp, got := 2048, tracer.steps; exp != got {
		t.Fatalf("expected %d steps, got %d", exp, got)
	}
}

func TestReturnSubShallow(t *testing.T) {
	state, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	address := common.HexToAddress("0x0a")
	// The code does returnsub without having anything on the returnstack.
	// It should not panic, but just fail after one step
	state.SetCode(address, []byte{
		byte(vm.PUSH1), 5,
		byte(vm.JUMPSUB),
		byte(vm.RETURNSUB),
		byte(vm.PC),
		byte(vm.BEGINSUB),
		byte(vm.RETURNSUB),
		byte(vm.PC),
	})
	tracer := stepCounter{}

	// Enable 2315
	_, _, err := Call(address, nil, &Config{State: state,
		GasLimit:    10000,
		ChainConfig: params.AllEthashProtocolChanges,
		EVMConfig: vm.Config{
			ExtraEips: []int{2315},
			Debug:     true,
			Tracer:    &tracer,
		}})

	exp := "invalid retsub"
	if err.Error() != exp {
		t.Fatalf("expected %v, got %v", exp, err)
	}
	if exp, got := 4, tracer.steps; exp != got {
		t.Fatalf("expected %d steps, got %d", exp, got)
	}
}

// disabled -- only used for generating markdown
func DisabledTestReturnCases(t *testing.T) {
	cfg := &Config{
		EVMConfig: vm.Config{
			Debug:     true,
			Tracer:    vm.NewMarkdownLogger(nil, os.Stdout),
			ExtraEips: []int{2315},
		},
	}
	// This should fail at first opcode
	Execute([]byte{
		byte(vm.RETURNSUB),
		byte(vm.PC),
		byte(vm.PC),
	}, nil, cfg)

	// Should also fail
	Execute([]byte{
		byte(vm.PUSH1), 5,
		byte(vm.JUMPSUB),
		byte(vm.RETURNSUB),
		byte(vm.PC),
		byte(vm.BEGINSUB),
		byte(vm.RETURNSUB),
		byte(vm.PC),
	}, nil, cfg)

	// This should complete
	Execute([]byte{
		byte(vm.PUSH1), 0x4,
		byte(vm.JUMPSUB),
		byte(vm.STOP),
		byte(vm.BEGINSUB),
		byte(vm.PUSH1), 0x9,
		byte(vm.JUMPSUB),
		byte(vm.RETURNSUB),
		byte(vm.BEGINSUB),
		byte(vm.RETURNSUB),
	}, nil, cfg)
}

// DisabledTestEipExampleCases contains various testcases that are used for the
// EIP examples
// This test is disabled, as it's only used for generating markdown
func DisabledTestEipExampleCases(t *testing.T) {
	cfg := &Config{
		EVMConfig: vm.Config{
			Debug:     true,
			Tracer:    vm.NewMarkdownLogger(nil, os.Stdout),
			ExtraEips: []int{2315},
		},
	}
	prettyPrint := func(comment string, code []byte) {
		instrs := make([]string, 0)
		it := asm.NewInstructionIterator(code)
		for it.Next() {
			if it.Arg() != nil && 0 < len(it.Arg()) {
				instrs = append(instrs, fmt.Sprintf("%v 0x%x", it.Op(), it.Arg()))
			} else {
				instrs = append(instrs, fmt.Sprintf("%v", it.Op()))
			}
		}
		ops := strings.Join(instrs, ", ")

		fmt.Printf("%v\nBytecode: `0x%x` (`%v`)\n",
			comment,
			code, ops)
		Execute(code, nil, cfg)
	}

	{ // First eip testcase
		code := []byte{
			byte(vm.PUSH1), 4,
			byte(vm.JUMPSUB),
			byte(vm.STOP),
			byte(vm.BEGINSUB),
			byte(vm.RETURNSUB),
		}
		prettyPrint("This should jump into a subroutine, back out and stop.", code)
	}

	{
		code := []byte{
			byte(vm.PUSH9), 0x00, 0x00, 0x00, 0x00, 0x0, 0x00, 0x00, 0x00, (4 + 8),
			byte(vm.JUMPSUB),
			byte(vm.STOP),
			byte(vm.BEGINSUB),
			byte(vm.PUSH1), 8 + 9,
			byte(vm.JUMPSUB),
			byte(vm.RETURNSUB),
			byte(vm.BEGINSUB),
			byte(vm.RETURNSUB),
		}
		prettyPrint("This should execute fine, going into one two depths of subroutines", code)
	}
	// TODO(@holiman) move this test into an actual test, which not only prints
	// out the trace.
	{
		code := []byte{
			byte(vm.PUSH9), 0x01, 0x00, 0x00, 0x00, 0x0, 0x00, 0x00, 0x00, (4 + 8),
			byte(vm.JUMPSUB),
			byte(vm.STOP),
			byte(vm.BEGINSUB),
			byte(vm.PUSH1), 8 + 9,
			byte(vm.JUMPSUB),
			byte(vm.RETURNSUB),
			byte(vm.BEGINSUB),
			byte(vm.RETURNSUB),
		}
		prettyPrint("This should fail, since the given location is outside of the "+
			"code-range. The code is the same as previous example, except that the "+
			"pushed location is `0x01000000000000000c` instead of `0x0c`.", code)
	}
	{
		// This should fail at first opcode
		code := []byte{
			byte(vm.RETURNSUB),
			byte(vm.PC),
			byte(vm.PC),
		}
		prettyPrint("This should fail at first opcode, due to shallow `return_stack`", code)

	}
	{
		code := []byte{
			byte(vm.PUSH1), 5, // Jump past the subroutine
			byte(vm.JUMP),
			byte(vm.BEGINSUB),
			byte(vm.RETURNSUB),
			byte(vm.JUMPDEST),
			byte(vm.PUSH1), 3, // Now invoke the subroutine
			byte(vm.JUMPSUB),
		}
		prettyPrint("In this example. the JUMPSUB is on the last byte of code. When the "+
			"subroutine returns, it should hit the 'virtual stop' _after_ the bytecode, "+
			"and not exit with error", code)
	}

	{
		code := []byte{
			byte(vm.BEGINSUB),
			byte(vm.RETURNSUB),
			byte(vm.STOP),
		}
		prettyPrint("In this example, the code 'walks' into a subroutine, which is not "+
			"allowed, and causes an error", code)
	}
}

// benchmarkNonModifyingCode benchmarks code, but if the code modifies the
// state, this should not be used, since it does not reset the state between runs.
func benchmarkNonModifyingCode(gas uint64, code []byte, name string, b *testing.B) {
	cfg := new(Config)
	setDefaults(cfg)
	cfg.State, _ = state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	cfg.GasLimit = gas
	var (
		destination = common.BytesToAddress([]byte("contract"))
		vmenv       = NewEnv(cfg)
		sender      = vm.AccountRef(cfg.Origin)
	)
	cfg.State.CreateAccount(destination)
	eoa := common.HexToAddress("E0")
	{
		cfg.State.CreateAccount(eoa)
		cfg.State.SetNonce(eoa, 100)
	}
	reverting := common.HexToAddress("EE")
	{
		cfg.State.CreateAccount(reverting)
		cfg.State.SetCode(reverting, []byte{
			byte(vm.PUSH1), 0x00,
			byte(vm.PUSH1), 0x00,
			byte(vm.REVERT),
		})
	}

	//cfg.State.CreateAccount(cfg.Origin)
	// set the receiver's (the executing contract) code for execution.
	cfg.State.SetCode(destination, code)
	vmenv.Call(sender, destination, nil, gas, cfg.Value)

	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			vmenv.Call(sender, destination, nil, gas, cfg.Value)
		}
	})
}

// BenchmarkSimpleLoop test a pretty simple loop which loops until OOG
// 55 ms
func BenchmarkSimpleLoop(b *testing.B) {

	staticCallIdentity := []byte{
		byte(vm.JUMPDEST), //  [ count ]
		// push args for the call
		byte(vm.PUSH1), 0, // out size
		byte(vm.DUP1),       // out offset
		byte(vm.DUP1),       // out insize
		byte(vm.DUP1),       // in offset
		byte(vm.PUSH1), 0x4, // address of identity
		byte(vm.GAS), // gas
		byte(vm.STATICCALL),
		byte(vm.POP),      // pop return value
		byte(vm.PUSH1), 0, // jumpdestination
		byte(vm.JUMP),
	}

	callIdentity := []byte{
		byte(vm.JUMPDEST), //  [ count ]
		// push args for the call
		byte(vm.PUSH1), 0, // out size
		byte(vm.DUP1),       // out offset
		byte(vm.DUP1),       // out insize
		byte(vm.DUP1),       // in offset
		byte(vm.DUP1),       // value
		byte(vm.PUSH1), 0x4, // address of identity
		byte(vm.GAS), // gas
		byte(vm.CALL),
		byte(vm.POP),      // pop return value
		byte(vm.PUSH1), 0, // jumpdestination
		byte(vm.JUMP),
	}

	callInexistant := []byte{
		byte(vm.JUMPDEST), //  [ count ]
		// push args for the call
		byte(vm.PUSH1), 0, // out size
		byte(vm.DUP1),        // out offset
		byte(vm.DUP1),        // out insize
		byte(vm.DUP1),        // in offset
		byte(vm.DUP1),        // value
		byte(vm.PUSH1), 0xff, // address of existing contract
		byte(vm.GAS), // gas
		byte(vm.CALL),
		byte(vm.POP),      // pop return value
		byte(vm.PUSH1), 0, // jumpdestination
		byte(vm.JUMP),
	}

	callEOA := []byte{
		byte(vm.JUMPDEST), //  [ count ]
		// push args for the call
		byte(vm.PUSH1), 0, // out size
		byte(vm.DUP1),        // out offset
		byte(vm.DUP1),        // out insize
		byte(vm.DUP1),        // in offset
		byte(vm.DUP1),        // value
		byte(vm.PUSH1), 0xE0, // address of EOA
		byte(vm.GAS), // gas
		byte(vm.CALL),
		byte(vm.POP),      // pop return value
		byte(vm.PUSH1), 0, // jumpdestination
		byte(vm.JUMP),
	}

	loopingCode := []byte{
		byte(vm.JUMPDEST), //  [ count ]
		// push args for the call
		byte(vm.PUSH1), 0, // out size
		byte(vm.DUP1),       // out offset
		byte(vm.DUP1),       // out insize
		byte(vm.DUP1),       // in offset
		byte(vm.PUSH1), 0x4, // address of identity
		byte(vm.GAS), // gas

		byte(vm.POP), byte(vm.POP), byte(vm.POP), byte(vm.POP), byte(vm.POP), byte(vm.POP),
		byte(vm.PUSH1), 0, // jumpdestination
		byte(vm.JUMP),
	}

	calllRevertingContractWithInput := []byte{
		byte(vm.JUMPDEST), //
		// push args for the call
		byte(vm.PUSH1), 0, // out size
		byte(vm.DUP1),        // out offset
		byte(vm.PUSH1), 0x20, // in size
		byte(vm.PUSH1), 0x00, // in offset
		byte(vm.PUSH1), 0x00, // value
		byte(vm.PUSH1), 0xEE, // address of reverting contract
		byte(vm.GAS), // gas
		byte(vm.CALL),
		byte(vm.POP),      // pop return value
		byte(vm.PUSH1), 0, // jumpdestination
		byte(vm.JUMP),
	}

	//tracer := vm.NewJSONLogger(nil, os.Stdout)
	//Execute(loopingCode, nil, &Config{
	//	EVMConfig: vm.Config{
	//		Debug:  true,
	//		Tracer: tracer,
	//	}})
	// 100M gas
	benchmarkNonModifyingCode(100000000, staticCallIdentity, "staticcall-identity-100M", b)
	benchmarkNonModifyingCode(100000000, callIdentity, "call-identity-100M", b)
	benchmarkNonModifyingCode(100000000, loopingCode, "loop-100M", b)
	benchmarkNonModifyingCode(100000000, callInexistant, "call-nonexist-100M", b)
	benchmarkNonModifyingCode(100000000, callEOA, "call-EOA-100M", b)
	benchmarkNonModifyingCode(100000000, calllRevertingContractWithInput, "call-reverting-100M", b)

	//benchmarkNonModifyingCode(10000000, staticCallIdentity, "staticcall-identity-10M", b)
	//benchmarkNonModifyingCode(10000000, loopingCode, "loop-10M", b)
}

// TestEip2929Cases contains various testcases that are used for
// EIP-2929 about gas repricings
func TestEip2929Cases(t *testing.T) {

	id := 1
	prettyPrint := func(comment string, code []byte) {

		instrs := make([]string, 0)
		it := asm.NewInstructionIterator(code)
		for it.Next() {
			if it.Arg() != nil && 0 < len(it.Arg()) {
				instrs = append(instrs, fmt.Sprintf("%v 0x%x", it.Op(), it.Arg()))
			} else {
				instrs = append(instrs, fmt.Sprintf("%v", it.Op()))
			}
		}
		ops := strings.Join(instrs, ", ")
		fmt.Printf("### Case %d\n\n", id)
		id++
		fmt.Printf("%v\n\nBytecode: \n```\n0x%x\n```\nOperations: \n```\n%v\n```\n\n",
			comment,
			code, ops)
		Execute(code, nil, &Config{
			EVMConfig: vm.Config{
				Debug:     true,
				Tracer:    vm.NewMarkdownLogger(nil, os.Stdout),
				ExtraEips: []int{2929},
			},
		})
	}

	{ // First eip testcase
		code := []byte{
			// Three checks against a precompile
			byte(vm.PUSH1), 1, byte(vm.EXTCODEHASH), byte(vm.POP),
			byte(vm.PUSH1), 2, byte(vm.EXTCODESIZE), byte(vm.POP),
			byte(vm.PUSH1), 3, byte(vm.BALANCE), byte(vm.POP),
			// Three checks against a non-precompile
			byte(vm.PUSH1), 0xf1, byte(vm.EXTCODEHASH), byte(vm.POP),
			byte(vm.PUSH1), 0xf2, byte(vm.EXTCODESIZE), byte(vm.POP),
			byte(vm.PUSH1), 0xf3, byte(vm.BALANCE), byte(vm.POP),
			// Same three checks (should be cheaper)
			byte(vm.PUSH1), 0xf2, byte(vm.EXTCODEHASH), byte(vm.POP),
			byte(vm.PUSH1), 0xf3, byte(vm.EXTCODESIZE), byte(vm.POP),
			byte(vm.PUSH1), 0xf1, byte(vm.BALANCE), byte(vm.POP),
			// Check the origin, and the 'this'
			byte(vm.ORIGIN), byte(vm.BALANCE), byte(vm.POP),
			byte(vm.ADDRESS), byte(vm.BALANCE), byte(vm.POP),

			byte(vm.STOP),
		}
		prettyPrint("This checks `EXT`(codehash,codesize,balance) of precompiles, which should be `100`, "+
			"and later checks the same operations twice against some non-precompiles. "+
			"Those are cheaper second time they are accessed. Lastly, it checks the `BALANCE` of `origin` and `this`.", code)
	}

	{ // EXTCODECOPY
		code := []byte{
			// extcodecopy( 0xff,0,0,0,0)
			byte(vm.PUSH1), 0x00, byte(vm.PUSH1), 0x00, byte(vm.PUSH1), 0x00, //length, codeoffset, memoffset
			byte(vm.PUSH1), 0xff, byte(vm.EXTCODECOPY),
			// extcodecopy( 0xff,0,0,0,0)
			byte(vm.PUSH1), 0x00, byte(vm.PUSH1), 0x00, byte(vm.PUSH1), 0x00, //length, codeoffset, memoffset
			byte(vm.PUSH1), 0xff, byte(vm.EXTCODECOPY),
			// extcodecopy( this,0,0,0,0)
			byte(vm.PUSH1), 0x00, byte(vm.PUSH1), 0x00, byte(vm.PUSH1), 0x00, //length, codeoffset, memoffset
			byte(vm.ADDRESS), byte(vm.EXTCODECOPY),

			byte(vm.STOP),
		}
		prettyPrint("This checks `extcodecopy( 0xff,0,0,0,0)` twice, (should be expensive first time), "+
			"and then does `extcodecopy( this,0,0,0,0)`.", code)
	}

	{ // SLOAD + SSTORE
		code := []byte{

			// Add slot `0x1` to access list
			byte(vm.PUSH1), 0x01, byte(vm.SLOAD), byte(vm.POP), // SLOAD( 0x1) (add to access list)
			// Write to `0x1` which is already in access list
			byte(vm.PUSH1), 0x11, byte(vm.PUSH1), 0x01, byte(vm.SSTORE), // SSTORE( loc: 0x01, val: 0x11)
			// Write to `0x2` which is not in access list
			byte(vm.PUSH1), 0x11, byte(vm.PUSH1), 0x02, byte(vm.SSTORE), // SSTORE( loc: 0x02, val: 0x11)
			// Write again to `0x2`
			byte(vm.PUSH1), 0x11, byte(vm.PUSH1), 0x02, byte(vm.SSTORE), // SSTORE( loc: 0x02, val: 0x11)
			// Read slot in access list (0x2)
			byte(vm.PUSH1), 0x02, byte(vm.SLOAD), // SLOAD( 0x2)
			// Read slot in access list (0x1)
			byte(vm.PUSH1), 0x01, byte(vm.SLOAD), // SLOAD( 0x1)
		}
		prettyPrint("This checks `sload( 0x1)` followed by `sstore(loc: 0x01, val:0x11)`, then 'naked' sstore:"+
			"`sstore(loc: 0x02, val:0x11)` twice, and `sload(0x2)`, `sload(0x1)`. ", code)
	}
	{ // Call variants
		code := []byte{
			// identity precompile
			byte(vm.PUSH1), 0x0, byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
			byte(vm.PUSH1), 0x04, byte(vm.PUSH1), 0x0, byte(vm.CALL), byte(vm.POP),

			// random account - call 1
			byte(vm.PUSH1), 0x0, byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
			byte(vm.PUSH1), 0xff, byte(vm.PUSH1), 0x0, byte(vm.CALL), byte(vm.POP),

			// random account - call 2
			byte(vm.PUSH1), 0x0, byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1), byte(vm.DUP1),
			byte(vm.PUSH1), 0xff, byte(vm.PUSH1), 0x0, byte(vm.STATICCALL), byte(vm.POP),
		}
		prettyPrint("This calls the `identity`-precompile (cheap), then calls an account (expensive) and `staticcall`s the same"+
			"account (cheap)", code)
	}
}
