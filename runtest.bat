@echo off
set tip="path.yml"
REM if -%1-==-path- set tip="%1.yml"
meqa.exe  -s=swagger.yaml  -v=true -r=meqa_data/result.yml -u=admin -w=admin 
