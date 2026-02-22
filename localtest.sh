#!/bin/bash

mkdir -p dist
go build -o dist/cfn main.go
mkdir -p $HOME/local/bin
mv dist/cfn $HOME/local/bin/cfn
cat <<EOF > $HOME/local/bin/cfn-list
#!/bin/bash
$HOME/local/bin/cfn list "\$@"
EOF
chmod +x $HOME/local/bin/cfn-list
