import {Capacitor,CapacitorHttp} from '@capacitor/core';
import {nativeAPIConfig} from './native-config.ts';
type Options={method:string;headers:Record<string,string>;body?:unknown;signal:AbortSignal};
export async function requestJSON(path:string,options:Options):Promise<{ok:boolean;status:number;data:unknown}>{
 if(!Capacitor.isNativePlatform()){
  const response=await fetch(`/api${path}`,{method:options.method,headers:options.headers,credentials:'same-origin',signal:options.signal,body:options.body===undefined?undefined:JSON.stringify(options.body)});
  const data=await response.json();return {ok:response.ok,status:response.status,data};
 }
 const env=import.meta.env;
 const config=nativeAPIConfig(env.VITE_NATIVE_API_URL,env.VITE_NATIVE_REQUEST_ORIGIN,env.MODE==='mock');
 if(options.signal.aborted)throw new DOMException('Aborted','AbortError');
 // Native URLSession owns HttpOnly cookies; tokens are never copied to JavaScript storage.
 const response=await CapacitorHttp.request({url:`${config.base}/api${path}`,method:options.method,headers:{...options.headers,Origin:config.origin},data:options.body,responseType:'json',connectTimeout:15000,readTimeout:30000,disableRedirects:true});
 // The native bridge cannot cancel URLSession; discard obsolete results before they reach Redux.
 if(options.signal.aborted)throw new DOMException('Aborted','AbortError');
 return {ok:response.status>=200&&response.status<300,status:response.status,data:response.data};
}
