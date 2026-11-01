import assert from 'node:assert/strict';
import {test} from 'node:test';
import {nativeAPIConfig} from '../web/src/native-config.ts';
test('native production API requires an explicit HTTPS origin',()=>{
 assert.deepEqual(nativeAPIConfig('https://dw.example',undefined),{base:'https://dw.example',origin:'https://dw.example'});
 for(const value of [undefined,'http://dw.example','http://127.0.0.1:8081','https://user:pass@dw.example','https://dw.example/api','https://dw.example/?token=secret'])assert.throws(()=>nativeAPIConfig(value,undefined));
});
test('native loopback HTTP is available only in explicit development builds',()=>{
 assert.deepEqual(nativeAPIConfig('http://127.0.0.1:8081','http://127.0.0.1:5173',true),{base:'http://127.0.0.1:8081',origin:'http://127.0.0.1:5173'});
 assert.throws(()=>nativeAPIConfig('http://192.168.1.2:8081',undefined,true));
 assert.throws(()=>nativeAPIConfig('https://dw.example','http://dw.example'));
});
