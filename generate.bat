@echo off
set tip="path"
if -%1-==-path- set tip="%1"
mqgo gen -s=swagger.yaml -a=%1% -v=false