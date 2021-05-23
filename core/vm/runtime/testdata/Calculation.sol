pragma solidity ^0.7.6;

contract Calculation {
    int a = 0;
    Add add;
    Sub sub;

    event Test(bytes32 t);
    constructor(int c){
        a = c;
        add = new Add();
        sub = new Sub();
    }

    function addNum(int b) public returns (int){
        a = add.add(a, b);
        return a;
    }

    function subNum(int aa) public returns (int){
        emit Test(sha256("subNum(int256)"));
        return sub.sub(aa);
    }

    function get() public view returns (int){
        return a;
    }
}

contract Add {
    function add(int a, int b) public pure returns (int c){
        return a + b;
    }
}

contract Sub {
    int a = 10;

    function sub(int b) public returns (int c){
        a = a - b;
        return a;
    }
}