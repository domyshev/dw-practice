import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
const base=`http://127.0.0.1:${process.env.MOCK_API_PORT||8081}`;
const origin=process.env.MOCK_APP_ORIGIN||`http://127.0.0.1:${process.env.MOCK_WEB_PORT||5174}`;
let cookie='';
async function api(path,method='GET',body,key){
 const response=await fetch(`${base}/api${path}`,{method,headers:{'Content-Type':'application/json',Origin:origin,...(cookie?{Cookie:cookie}:{}),...(key?{'Idempotency-Key':key}:{})},body:body===undefined?undefined:JSON.stringify(body)});
 const data=await response.json();assert.equal(response.ok,true,`${method} ${path}: ${JSON.stringify(data)}`);
 const next=response.headers.get('set-cookie');if(next)cookie=next.split(';')[0];return data;
}
const health=await api('/health');assert.equal(health.mock,true,'This smoke check only runs against the mock API');
const email=`smoke-${randomUUID()}@dwpractice.test`;
await api('/auth/request','POST',{email});await api('/auth/verify','POST',{email,code:'123456'});
assert.equal((await api('/me')).email,email);assert.deepEqual(await api('/entries'),[]);
const body={practice:'mandala',large:2,small:1,repetitions:4,initial:false,occurredAt:new Date(Date.now()-60000).toISOString(),timeZone:'UTC',note:'Mock API smoke check',version:0};
const key=randomUUID();const created=await api('/entries','POST',body,key);assert.equal(created.entry.total,247);
assert.equal((await api('/entries','POST',body,key)).entry.id,created.entry.id);
const updated=await api(`/entries/${created.entry.id}`,'PUT',{...body,repetitions:5,version:1});assert.equal(updated.entry.total,248);
await api(`/entries/${created.entry.id}?version=2`,'DELETE');assert.deepEqual(await api('/entries'),[]);
assert.deepEqual((await api('/audit')).map(e=>e.action),['deleted','updated','created']);
await api('/auth/logout','POST');console.log('Mock smoke passed: login, empty account, create, idempotent retry, edit, delete, audit, logout.');
