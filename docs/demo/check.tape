# Records docs/images/check.gif:  vhs docs/demo/check.tape
# Needs ./bin/oraclewatch (make build) and network access to public RPCs.
Output docs/images/check.gif

Set Shell "bash"
Set FontSize 15
Set Width 1400
Set Height 900
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Hide
Type "export PATH=$PWD/bin:$PATH DEMO_ETH_RPC_A=https://ethereum-rpc.publicnode.com DEMO_ETH_RPC_B=https://eth.drpc.org DEMO_ARB_RPC_A=https://arbitrum-one-rpc.publicnode.com DEMO_ARB_RPC_B=https://arb1.arbitrum.io/rpc && clear" Enter
Sleep 500ms
Show

Type "oraclewatch validate-config --config docs/demo/demo.yaml" Sleep 400ms Enter
Sleep 1500ms
Type "oraclewatch check --config docs/demo/demo.yaml --fail-on warning; echo exit=$?" Sleep 400ms Enter
Sleep 25s
