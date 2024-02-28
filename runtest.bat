@echo off
set tip="simple.yml"
if -%1-==-path- set tip="%1.yml"
mqgo.exe run -s=swagger.yaml -p=meqa_data/%tip% -v=true -r=meqa_data/result.yml -u=admin -w=admin
