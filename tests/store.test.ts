import assert from 'node:assert/strict';
import {afterEach,beforeEach,test} from 'node:test';
import {actions,api,mutateJournal,reload,SessionChangedError,store,type Entry,type Audit} from '../web/src/store.ts';
const originalFetch=globalThis.fetch;
const userA={id:'a',email:'a@example.com'},userB={id:'b',email:'b@example.com'};
const entry=(id:string,userId:string):Entry=>({id,userId,practice:'mandala',large:1,small:0,repetitions:0,total:108,initial:false,occurredAt:'2026-01-20T20:15:00+01:00',timeZone:'Europe/Madrid',offsetMinutes:60,note:'',createdAt:'2026-01-20T19:15:00Z',updatedAt:'2026-01-20T19:15:00Z',version:1});
const audit=(e:Entry):Audit=>({id:e.id,entryId:e.id,action:'created',at:e.createdAt,before:null,after:e});
const response=(body:unknown,status=200)=>new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}});
function deferred<T>(){let resolve!:(v:T)=>void;const promise=new Promise<T>(r=>{resolve=r});return{promise,resolve};}
beforeEach(()=>store.dispatch(actions.signedOut()));
afterEach(()=>{globalThis.fetch=originalFetch;store.dispatch(actions.signedOut())});

test('late journal and audit responses cannot cross account boundaries',async()=>{
 store.dispatch(actions.signedIn(userA));
 const oldEntries=deferred<Response>(),oldAudit=deferred<Response>();
 globalThis.fetch=async path=>String(path).endsWith('/entries')?oldEntries.promise:oldAudit.promise;
 const oldLoad=reload();
 store.dispatch(actions.signedOut());store.dispatch(actions.signedIn(userB));
 const b=entry('b-entry','b');
 globalThis.fetch=async path=>response(String(path).endsWith('/entries')?[b]:[audit(b)]);
 await reload();
 const a=entry('a-entry','a');oldEntries.resolve(response([a]));oldAudit.resolve(response([audit(a)]));await oldLoad;
 assert.deepEqual(store.getState().entries,[b]);assert.deepEqual(store.getState().audit,[audit(b)]);assert.equal(store.getState().user?.id,'b');
});

test('an obsolete 401 cannot sign out a newly authenticated user',async()=>{
 store.dispatch(actions.signedIn(userA));const pending=deferred<Response>();globalThis.fetch=()=>pending.promise;
 const request=api('/entries');const rejected=assert.rejects(request,SessionChangedError);
 store.dispatch(actions.signedOut());store.dispatch(actions.signedIn(userB));pending.resolve(response({error:'Please sign in'},401));await rejected;
 assert.equal(store.getState().user?.id,'b');
});

test('logout clears data and late successful responses cannot restore it',async()=>{
 store.dispatch(actions.signedIn(userA));const pending=deferred<Response>();globalThis.fetch=()=>pending.promise;const load=reload();
 store.dispatch(actions.signedOut());pending.resolve(response([entry('private','a')]));await load;
 assert.equal(store.getState().user,null);assert.deepEqual(store.getState().entries,[]);assert.deepEqual(store.getState().audit,[]);
});

test('a new session for the same account still invalidates previous requests',async()=>{
 store.dispatch(actions.signedIn(userA));const pending=deferred<Response>();globalThis.fetch=()=>pending.promise;const load=reload();
 store.dispatch(actions.signedIn(userA));pending.resolve(response([entry('old','a')]));await load;
 assert.deepEqual(store.getState().entries,[]);
});

test('a slower load cannot overwrite a newer load in the same session',async()=>{
 store.dispatch(actions.signedIn(userA));const first=deferred<Response>();globalThis.fetch=()=>first.promise;const oldLoad=reload();
 const fresh=entry('fresh','a');globalThis.fetch=async path=>response(String(path).endsWith('/entries')?[fresh]:[]);await reload();
 first.resolve(response([]));await oldLoad;assert.deepEqual(store.getState().entries,[fresh]);
});

test('save commits and closes before refresh, and retrying a failed load never repeats POST',async()=>{
 store.dispatch(actions.signedIn(userA));let posts=0,closed=false,failRefresh=true;const saved=entry('saved','a');
 globalThis.fetch=async(path,options)=>{
  if(options?.method==='POST'){posts++;assert.equal((options.headers as Record<string,string>)['Idempotency-Key'],'operation-1');return response({entry:saved},201)}
  assert.equal(closed,true,'Editor must close before refreshing');
  if(failRefresh)return response({error:'Temporarily unavailable'},503);
  return response(String(path).endsWith('/entries')?[saved]:[audit(saved)]);
 };
 await mutateJournal('/entries','POST',{},()=>{closed=true},'operation-1');
 assert.equal(closed,true);assert.equal(posts,1);assert.deepEqual(store.getState().entries,[saved]);assert.equal(store.getState().refreshError,'Temporarily unavailable');
 failRefresh=false;await reload();assert.equal(posts,1);assert.equal(store.getState().refreshError,'');assert.deepEqual(store.getState().audit,[audit(saved)]);
});

test('a failed mutation does not close the editor or refresh the journal',async()=>{
 store.dispatch(actions.signedIn(userA));let committed=false,calls=0;globalThis.fetch=async()=>{calls++;return response({error:'Write failed'},500)};
 await assert.rejects(mutateJournal('/entries','POST',{},()=>{committed=true}),/Write failed/);
 assert.equal(committed,false);assert.equal(calls,1);assert.deepEqual(store.getState().entries,[]);
});

test('late mutation results cannot update or close another account journal',async()=>{
 store.dispatch(actions.signedIn(userA));const pending=deferred<Response>();globalThis.fetch=()=>pending.promise;let closed=false;
 const mutation=mutateJournal('/entries','POST',{},()=>{closed=true});const rejected=assert.rejects(mutation,SessionChangedError);
 store.dispatch(actions.signedIn(userB));pending.resolve(response({entry:entry('a-entry','a')},201));await rejected;
 assert.equal(closed,false);assert.equal(store.getState().user?.id,'b');assert.deepEqual(store.getState().entries,[]);
});

test('a session transition aborts requests that support cancellation',async()=>{
 store.dispatch(actions.signedIn(userA));let aborted=false;
 globalThis.fetch=(_path,options)=>new Promise((_resolve,reject)=>options?.signal?.addEventListener('abort',()=>{aborted=true;reject(new DOMException('Aborted','AbortError'))}));
 const request=api('/entries');const rejected=assert.rejects(request,SessionChangedError);store.dispatch(actions.signedOut());await rejected;assert.equal(aborted,true);
});

test('a delayed old load failure cannot replace the new account refresh error',async()=>{
 store.dispatch(actions.signedIn(userA));const pending=deferred<Response>();globalThis.fetch=()=>pending.promise;const oldLoad=reload();
 store.dispatch(actions.signedIn(userB));globalThis.fetch=async()=>response({error:'New account refresh failed'},503);await reload();
 pending.resolve(response({error:'Old account failure'},500));await oldLoad;
 assert.equal(store.getState().refreshError,'New account refresh failed');
});
