package runtime

import (
	"crypto/sha256"
	"encoding/json"
	"evm/abi"
	"evm/abi/bind"
	"evm/common"
	"evm/common/compiler"
	"evm/core/rawdb"
	"evm/core/state"
	"evm/crypto"
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/big"
	"strings"
	"testing"
)

//模拟stateRoot进行存储
var rootHashKey = []byte("stateRoot")
var dbUrl = "./testdata/testdb"
var contractName = "Calculation"

var contractSource = `
pragma solidity ^0.7.6;

contract Calculation{

   int a = 0;
   event Test(int indexed a,string indexed b,int d);
   constructor(int c){
       a = c;
   }

   function add(int b) public returns(int){
       a += b;
       emit Test(a,"hello world",1);
       return a;
   }
}`

//内存测试编译合约、部署合约、调用合约、发送日志流程
func TestCreateContractAndInvokeInMemory(t *testing.T) {
	//编译合约
	contract := compileContract("")
	code := strings.TrimPrefix(contract.Code, "0x")
	//初始化合约参数（构造方法的入参）
	abiBytes, err := json.Marshal(contract.Info.AbiDefinition)
	assert.NoError(t, err)
	contractAbi, err := abi.JSON(strings.NewReader(string(abiBytes)))
	assert.NoError(t, err)
	byteInput, err := contractAbi.Pack("", big.NewInt(1))
	assert.NoError(t, err)

	input := common.Bytes2Hex(byteInput)
	codeAndInput := common.Hex2Bytes(code + input)
	cfg := new(Config)
	//合约代码，合约地址，剩余的gas，错误
	returnCode, address, _, err := Create(codeAndInput, cfg)
	assert.NoError(t, err)
	//断言返回的code是正确的
	hexCode := common.Bytes2Hex(returnCode)
	assert.True(t, strings.Contains(code, hexCode))
	fmt.Println(fmt.Sprintf("合约字节码：%s", hexCode))

	//编译调用合约的abi
	for method := range contractAbi.Methods {
		byteInput, err = contractAbi.Pack(method, big.NewInt(2))
		assert.NoError(t, err)
	}

	//合约返回的结果，剩余的gas，错误
	result, _, err := Call(address, byteInput, cfg)
	assert.NoError(t, err)
	fmt.Println(fmt.Sprintf("创建合约执行结果：%s", common.Bytes2Hex(result)))

	fmt.Println("--------------打印合约日志---------------")

	boundContract := bind.NewBoundContract(cfg.Origin, contractAbi, nil, nil, nil)
	logs := cfg.State.Logs()
	if logs != nil && len(logs) > 0 {
		for i, log := range logs {
			//topics 第一个 为事件签名的 Keccak256Hash 的hex
			assert.Equal(t, crypto.Keccak256Hash([]byte(contractAbi.Events["Test"].Sig)).Hex(), log.Topics[0].String())
			assert.Equal(t, contractAbi.Events["Test"].ID.Hex(), log.Topics[0].String())

			fmt.Println(fmt.Sprintf("log%d : %v", i, log))
			received := make(map[string]interface{})
			err = boundContract.UnpackLogIntoMap(received, "Test", *log)
			assert.NoError(t, err)
			fmt.Println(received)
		}
	}
}

//内存测试编译合约、调用预编译合约、合约调用合约、发送日志等
func TestContarct2ContractInMemory(t *testing.T) {
	//编译合约
	code := compileContractCode("./testdata/Calculation.sol")
	//初始化合约参数（构造方法的入参）
	input := "0000000000000000000000000000000000000000000000000000000000000001"
	codeAndInput := common.Hex2Bytes(code + input)
	cfg := new(Config)
	cfg.BlockNumber = new(big.Int).SetInt64(1)
	//合约代码，合约地址，剩余的gas，错误
	_, address, _, err := Create(codeAndInput, cfg)
	assert.NoError(t, err)

	//调用加法
	//inputBytes := common.Hex2Bytes("cc13962a0000000000000000000000000000000000000000000000000000000000000002")

	//调用减法
	inputBytes := common.Hex2Bytes("afa6cbdd0000000000000000000000000000000000000000000000000000000000000001")
	//合约返回的结果，剩余的gas，错误
	result, _, err := Call(address, inputBytes, cfg)
	assert.NoError(t, err)
	fmt.Println(fmt.Sprintf("创建合约执行结果：%s", common.Bytes2Hex(result)))

	fmt.Println("--------------打印合约日志---------------")

	logs := cfg.State.Logs()
	if logs != nil && len(logs) > 0 {
		for i, log := range logs {
			fmt.Println(fmt.Sprintf("log%d : %v", i, log))

			fmt.Println("--------断言调用的预编译合约执行 sha256 的正确性------------")
			sum := sha256.Sum256([]byte("subNum(int256)"))
			assert.Equal(t, log.Data, sum[:])
			fmt.Println(common.BytesToHash(log.Data).Hex())
			fmt.Println(common.BytesToHash(sum[:]).Hex())
		}
	}
}

//使用levelDB进行合约数据存储。若本地测试，请勿提交 core/vm/runtime/testdata/testdb 提交
func TestDeployContract(t *testing.T) {
	//编译合约
	code := compileContractCode("")
	input := "0000000000000000000000000000000000000000000000000000000000000001"
	codeAndInput := common.Hex2Bytes(code + input)

	//构建配置
	cfg := buildConfig(dbUrl)
	//合约代码，合约地址，剩余的gas，错误
	returnCode, address, _, err := Create(codeAndInput, cfg)

	assert.NoError(t, err)
	//断言返回的code是正确的
	hexCode := common.Bytes2Hex(returnCode)
	assert.True(t, strings.Contains(code, hexCode))
	fmt.Println(fmt.Sprintf("合约字节码：%s", hexCode))
	fmt.Println(fmt.Sprintf("合约地址 ： %s", common.Bytes2Hex(address.Bytes())))

	//将state提交到数据库
	commit(cfg.State)
}

//使用levelDB进行合约数据存储。若本地测试，请勿提交 core/vm/runtime/testdata/testdb 提交
//调用合约
func TestInvokeContract(t *testing.T) {
	//构建配置
	cfg := buildConfig(dbUrl)

	address := common.BytesToAddress(common.Hex2Bytes("bd770416a3345f91e4b34576cb804a576fa48eb1"))
	inputBytes := common.Hex2Bytes("87db03b70000000000000000000000000000000000000000000000000000000000000001")
	//合约返回的结果，剩余的gas，错误
	result, _, err := Call(address, inputBytes, cfg)
	assert.NoError(t, err)

	//将state提交到数据库
	commit(cfg.State)

	fmt.Println(fmt.Sprintf("合约执行结果：%s", common.Bytes2Hex(result)))
}

//测试快照，执行后进行快照回滚。可用于查询合约或者模拟执行，不会更改状态数据
func TestSnapshort(t *testing.T) {
	//构建配置
	cfg := buildConfig(dbUrl)

	address := common.BytesToAddress(common.Hex2Bytes("bd770416a3345f91e4b34576cb804a576fa48eb1"))
	inputBytes := common.Hex2Bytes("87db03b70000000000000000000000000000000000000000000000000000000000000001")

	snapshot := cfg.State.Snapshot()

	//合约返回的结果，剩余的gas，错误
	result, _, _ := Call(address, inputBytes, cfg)

	fmt.Println(fmt.Sprintf("合约执行结果：%s", common.Bytes2Hex(result)))
	cfg.State.RevertToSnapshot(snapshot)
}

func buildConfig(dbUrl string) *Config {
	cfg := new(Config)
	levelDB, err := rawdb.NewLevelDBDatabase(dbUrl, 0, 0, "bradyDB")
	if err != nil {
		panic(fmt.Sprintf("无法创建或打开db。 url : %s", dbUrl))
	}

	root, _ := levelDB.Get(rootHashKey)

	roothash := common.Hash{}
	if root != nil {
		roothash = common.BytesToHash(root)
	}

	stateDB, err := state.New(roothash, state.NewDatabase(levelDB), nil)
	cfg.State = stateDB

	return cfg
}

func commit(stateDB *state.StateDB) {
	//刷新到底层Tire
	root, _ := stateDB.Commit(true)
	db := stateDB.Database().TrieDB()
	//将Tire树到所有节点刷新到磁盘（DB）
	_ = db.Commit(root, false, nil)

	err := stateDB.Database().TrieDB().DiskDB().Put(rootHashKey, root.Bytes())
	if err != nil {
		panic("commit root hash error。")
	}
}

func compileContractCode(contractSourcePath string) string {
	c := compileContract(contractSourcePath)
	compileCode := c.Code
	return strings.TrimPrefix(compileCode, "0x")
}

func compileContract(contractSourcePath string) *compiler.Contract {
	var contracts map[string]*compiler.Contract
	var err error
	var contract = contractName
	if len(contractSourcePath) > 0 {
		contracts, err = compiler.CompileSolidity("", contractSourcePath)
		contract = contractSourcePath + ":" + contractName
	} else {
		contracts, err = compiler.CompileSolidityString("", contractSource)
	}
	if err != nil {
		panic("无法编译合约源码")
	}

	c, ok := contracts[contract]
	if !ok {
		contract = "<stdin>:" + contract
		c, ok = contracts[contract]
		if !ok {
			panic(fmt.Sprintf("未找到 %s 合约", contract))
		}
	}
	return c
}
