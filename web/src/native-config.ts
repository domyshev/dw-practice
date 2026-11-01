export type NativeAPIConfig={base:string;origin:string};
export function nativeAPIConfig(base:unknown,origin:unknown,development=false):NativeAPIConfig{
 if(typeof base!=='string'||!base)throw new Error('The iPhone build is missing its API address. Set VITE_NATIVE_API_URL and rebuild.');
 const url=new URL(base);
 const loopback=['localhost','127.0.0.1','[::1]'].includes(url.hostname);
 if(url.username||url.password||url.search||url.hash||url.pathname!=='/')throw new Error('The API address must be an origin without a path, credentials or query.');
 if(url.protocol!=='https:'&&!(development&&loopback&&url.protocol==='http:'))throw new Error('The iPhone API requires HTTPS. HTTP is allowed only for loopback development.');
 const requestOrigin=typeof origin==='string'&&origin?new URL(origin):url;
 if(requestOrigin.username||requestOrigin.password||requestOrigin.pathname!=='/'||requestOrigin.search||requestOrigin.hash||!['http:','https:'].includes(requestOrigin.protocol))throw new Error('Invalid native request origin.');
 if(!development&&requestOrigin.protocol!=='https:')throw new Error('The production request origin requires HTTPS.');
 return {base:url.origin,origin:requestOrigin.origin};
}
