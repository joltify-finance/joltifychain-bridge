1. prepare the sol file
2. run `solc --abi oppyTransfer.sol -o abi `
3. run `solc --bin oppyTransfer.sol -o bin `
4. abigen --abi=abi/EvmOppyBridge.abi --bin=bin/EvmOppyBridge.bin --pkg generated --out=oppy_transfer.go
