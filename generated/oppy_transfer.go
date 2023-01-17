//go:build !coverage
// +build !coverage

// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package generated

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// GeneratedMetaData contains all meta data concerning the Generated contract.
var GeneratedMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"contractAddress\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"memo\",\"type\":\"bytes\"}],\"name\":\"OppyTransfer\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"toAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"contractAddress\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"memo\",\"type\":\"bytes\"}],\"name\":\"oppyTransfer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x608060405234801561001057600080fd5b506105f2806100206000396000f3fe608060405234801561001057600080fd5b506004361061002b5760003560e01c8063407253be14610030575b600080fd5b61004a600480360381019061004591906102d7565b61004c565b005b61006b8373ffffffffffffffffffffffffffffffffffffffff166101b1565b6100aa576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016100a1906103bc565b60405180910390fd5b8273ffffffffffffffffffffffffffffffffffffffff166323b872dd3387876040518463ffffffff1660e01b81526004016100e7939291906103fa565b6020604051808303816000875af1158015610106573d6000803e3d6000fd5b505050506040513d601f19601f8201168201806040525081019061012a9190610469565b610169576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401610160906104e2565b60405180910390fd5b7fd007b7977d1c9492374ca3858085ea3ad2f931d034eb39f4889595196894fee03386868686866040516101a296959493929190610560565b60405180910390a15050505050565b6000808273ffffffffffffffffffffffffffffffffffffffff163b119050919050565b600080fd5b600080fd5b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000610209826101de565b9050919050565b610219816101fe565b811461022457600080fd5b50565b60008135905061023681610210565b92915050565b6000819050919050565b61024f8161023c565b811461025a57600080fd5b50565b60008135905061026c81610246565b92915050565b600080fd5b600080fd5b600080fd5b60008083601f84011261029757610296610272565b5b8235905067ffffffffffffffff8111156102b4576102b3610277565b5b6020830191508360018202830111156102d0576102cf61027c565b5b9250929050565b6000806000806000608086880312156102f3576102f26101d4565b5b600061030188828901610227565b95505060206103128882890161025d565b945050604061032388828901610227565b935050606086013567ffffffffffffffff811115610344576103436101d9565b5b61035088828901610281565b92509250509295509295909350565b600082825260208201905092915050565b7f6e6f74206120636f6e7472616374206164647265737300000000000000000000600082015250565b60006103a660168361035f565b91506103b182610370565b602082019050919050565b600060208201905081810360008301526103d581610399565b9050919050565b6103e5816101fe565b82525050565b6103f48161023c565b82525050565b600060608201905061040f60008301866103dc565b61041c60208301856103dc565b61042960408301846103eb565b949350505050565b60008115159050919050565b61044681610431565b811461045157600080fd5b50565b6000815190506104638161043d565b92915050565b60006020828403121561047f5761047e6101d4565b5b600061048d84828501610454565b91505092915050565b7f7472616e73666572206661696c65640000000000000000000000000000000000600082015250565b60006104cc600f8361035f565b91506104d782610496565b602082019050919050565b600060208201905081810360008301526104fb816104bf565b9050919050565b600082825260208201905092915050565b82818337600083830152505050565b6000601f19601f8301169050919050565b600061053f8385610502565b935061054c838584610513565b61055583610522565b840190509392505050565b600060a08201905061057560008301896103dc565b61058260208301886103dc565b61058f60408301876103eb565b61059c60608301866103dc565b81810360808301526105af818486610533565b905097965050505050505056fea2646970667358221220c794f6ac2b964c346b082074634e8060f39060e5c477e59ee47af4d2c0d10a8864736f6c634300080b0033",
}

// GeneratedABI is the input ABI used to generate the binding from.
// Deprecated: Use GeneratedMetaData.ABI instead.
var GeneratedABI = GeneratedMetaData.ABI

// GeneratedBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use GeneratedMetaData.Bin instead.
var GeneratedBin = GeneratedMetaData.Bin

// DeployGenerated deploys a new Ethereum contract, binding an instance of Generated to it.
func DeployGenerated(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *Generated, error) {
	parsed, err := GeneratedMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(GeneratedBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &Generated{GeneratedCaller: GeneratedCaller{contract: contract}, GeneratedTransactor: GeneratedTransactor{contract: contract}, GeneratedFilterer: GeneratedFilterer{contract: contract}}, nil
}

// Generated is an auto generated Go binding around an Ethereum contract.
type Generated struct {
	GeneratedCaller     // Read-only binding to the contract
	GeneratedTransactor // Write-only binding to the contract
	GeneratedFilterer   // Log filterer for contract events
}

// GeneratedCaller is an auto generated read-only Go binding around an Ethereum contract.
type GeneratedCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GeneratedTransactor is an auto generated write-only Go binding around an Ethereum contract.
type GeneratedTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GeneratedFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type GeneratedFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GeneratedSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type GeneratedSession struct {
	Contract     *Generated        // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// GeneratedCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type GeneratedCallerSession struct {
	Contract *GeneratedCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts    // Call options to use throughout this session
}

// GeneratedTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type GeneratedTransactorSession struct {
	Contract     *GeneratedTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// GeneratedRaw is an auto generated low-level Go binding around an Ethereum contract.
type GeneratedRaw struct {
	Contract *Generated // Generic contract binding to access the raw methods on
}

// GeneratedCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type GeneratedCallerRaw struct {
	Contract *GeneratedCaller // Generic read-only contract binding to access the raw methods on
}

// GeneratedTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type GeneratedTransactorRaw struct {
	Contract *GeneratedTransactor // Generic write-only contract binding to access the raw methods on
}

// NewGenerated creates a new instance of Generated, bound to a specific deployed contract.
func NewGenerated(address common.Address, backend bind.ContractBackend) (*Generated, error) {
	contract, err := bindGenerated(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Generated{GeneratedCaller: GeneratedCaller{contract: contract}, GeneratedTransactor: GeneratedTransactor{contract: contract}, GeneratedFilterer: GeneratedFilterer{contract: contract}}, nil
}

// NewGeneratedCaller creates a new read-only instance of Generated, bound to a specific deployed contract.
func NewGeneratedCaller(address common.Address, caller bind.ContractCaller) (*GeneratedCaller, error) {
	contract, err := bindGenerated(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &GeneratedCaller{contract: contract}, nil
}

// NewGeneratedTransactor creates a new write-only instance of Generated, bound to a specific deployed contract.
func NewGeneratedTransactor(address common.Address, transactor bind.ContractTransactor) (*GeneratedTransactor, error) {
	contract, err := bindGenerated(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &GeneratedTransactor{contract: contract}, nil
}

// NewGeneratedFilterer creates a new log filterer instance of Generated, bound to a specific deployed contract.
func NewGeneratedFilterer(address common.Address, filterer bind.ContractFilterer) (*GeneratedFilterer, error) {
	contract, err := bindGenerated(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &GeneratedFilterer{contract: contract}, nil
}

// bindGenerated binds a generic wrapper to an already deployed contract.
func bindGenerated(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(GeneratedABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Generated *GeneratedRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Generated.Contract.GeneratedCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Generated *GeneratedRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Generated.Contract.GeneratedTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Generated *GeneratedRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Generated.Contract.GeneratedTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Generated *GeneratedCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Generated.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Generated *GeneratedTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Generated.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Generated *GeneratedTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Generated.Contract.contract.Transact(opts, method, params...)
}

// OppyTransfer is a paid mutator transaction binding the contract method 0x407253be.
//
// Solidity: function oppyTransfer(address toAddress, uint256 amount, address contractAddress, bytes memo) returns()
func (_Generated *GeneratedTransactor) OppyTransfer(opts *bind.TransactOpts, toAddress common.Address, amount *big.Int, contractAddress common.Address, memo []byte) (*types.Transaction, error) {
	return _Generated.contract.Transact(opts, "oppyTransfer", toAddress, amount, contractAddress, memo)
}

// OppyTransfer is a paid mutator transaction binding the contract method 0x407253be.
//
// Solidity: function oppyTransfer(address toAddress, uint256 amount, address contractAddress, bytes memo) returns()
func (_Generated *GeneratedSession) OppyTransfer(toAddress common.Address, amount *big.Int, contractAddress common.Address, memo []byte) (*types.Transaction, error) {
	return _Generated.Contract.OppyTransfer(&_Generated.TransactOpts, toAddress, amount, contractAddress, memo)
}

// OppyTransfer is a paid mutator transaction binding the contract method 0x407253be.
//
// Solidity: function oppyTransfer(address toAddress, uint256 amount, address contractAddress, bytes memo) returns()
func (_Generated *GeneratedTransactorSession) OppyTransfer(toAddress common.Address, amount *big.Int, contractAddress common.Address, memo []byte) (*types.Transaction, error) {
	return _Generated.Contract.OppyTransfer(&_Generated.TransactOpts, toAddress, amount, contractAddress, memo)
}

// GeneratedOppyTransferIterator is returned from FilterOppyTransfer and is used to iterate over the raw logs and unpacked data for OppyTransfer events raised by the Generated contract.
type GeneratedOppyTransferIterator struct {
	Event *GeneratedOppyTransfer // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GeneratedOppyTransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GeneratedOppyTransfer)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GeneratedOppyTransfer)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GeneratedOppyTransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GeneratedOppyTransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GeneratedOppyTransfer represents a OppyTransfer event raised by the Generated contract.
type GeneratedOppyTransfer struct {
	From            common.Address
	To              common.Address
	Amount          *big.Int
	ContractAddress common.Address
	Memo            []byte
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterOppyTransfer is a free log retrieval operation binding the contract event 0xd007b7977d1c9492374ca3858085ea3ad2f931d034eb39f4889595196894fee0.
//
// Solidity: event OppyTransfer(address from, address to, uint256 amount, address contractAddress, bytes memo)
func (_Generated *GeneratedFilterer) FilterOppyTransfer(opts *bind.FilterOpts) (*GeneratedOppyTransferIterator, error) {

	logs, sub, err := _Generated.contract.FilterLogs(opts, "OppyTransfer")
	if err != nil {
		return nil, err
	}
	return &GeneratedOppyTransferIterator{contract: _Generated.contract, event: "OppyTransfer", logs: logs, sub: sub}, nil
}

// WatchOppyTransfer is a free log subscription operation binding the contract event 0xd007b7977d1c9492374ca3858085ea3ad2f931d034eb39f4889595196894fee0.
//
// Solidity: event OppyTransfer(address from, address to, uint256 amount, address contractAddress, bytes memo)
func (_Generated *GeneratedFilterer) WatchOppyTransfer(opts *bind.WatchOpts, sink chan<- *GeneratedOppyTransfer) (event.Subscription, error) {

	logs, sub, err := _Generated.contract.WatchLogs(opts, "OppyTransfer")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GeneratedOppyTransfer)
				if err := _Generated.contract.UnpackLog(event, "OppyTransfer", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOppyTransfer is a log parse operation binding the contract event 0xd007b7977d1c9492374ca3858085ea3ad2f931d034eb39f4889595196894fee0.
//
// Solidity: event OppyTransfer(address from, address to, uint256 amount, address contractAddress, bytes memo)
func (_Generated *GeneratedFilterer) ParseOppyTransfer(log types.Log) (*GeneratedOppyTransfer, error) {
	event := new(GeneratedOppyTransfer)
	if err := _Generated.contract.UnpackLog(event, "OppyTransfer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
