const cases = [];
function test(name, body) { cases.push({name, body}); }
const assert = {
 equal(actual, expected) {
  if (!Object.is(actual,expected)) throw new Error(`expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
 },
 deepEqual(actual, expected) { this.equal(JSON.stringify(actual), JSON.stringify(expected)); },
 match(actual, pattern) { if (!pattern.test(actual)) throw new Error(`expected ${JSON.stringify(actual)} to match ${pattern}`); },
};
class URLSearchParams {
 constructor(entries) { this.entries = Array.from(entries, ([key,value])=>[String(key),String(value)]); }
 set(key,value) {
  const first = this.entries.findIndex(entry=>entry[0]===key);
  this.entries=this.entries.filter((entry,index)=>entry[0]!==key || index===first);
  if(first<0) this.entries.push([key,String(value)]); else this.entries[first]=[key,String(value)];
 }
 toString() {
  const encode = value=>encodeURIComponent(value).replace(/[!'()~]/g, character=>'%'+character.charCodeAt(0).toString(16).toUpperCase()).replace(/%20/g,'+');
  return this.entries.map(([key,value])=>encode(key)+'='+encode(value)).join('&');
 }
}

function bridge(overrides={}) {
 const listeners = new Map();
 const document = {
  addEventListener(name, fn) { const entries=listeners.get(name)||[]; entries.push(fn); listeners.set(name,entries); },
  querySelectorAll() {return [];}, querySelector() {return null;}, getElementById() {return null;},
  body:{dataset:{}}, activeElement:null,
 };
 const window = {addEventListener(){}};
 const FormData = class {constructor(form){this.entries=form.values||[];} [Symbol.iterator](){return this.entries[Symbol.iterator]();}};
 const instantiate = new Function('document','window','htmx','setTimeout','clearTimeout','URLSearchParams','FormData','localStorage',bridgeSource);
 instantiate(document,window,{trigger(){},...overrides},()=>1,()=>{},URLSearchParams,FormData,{getItem(){return null;}});
 return {document,window, emit(name,event={}) {for(const fn of listeners.get(name)||[]) fn(event);}};
}
for (const composition of ['global','event','legacy']) {
 test(`IME Enter leaves composition untouched (${composition})`,()=>{
  const app=bridge(); let prevented=false, advanced=false;
  const target={matches(selector){return selector==='input[data-enter-next]';}};
  app.document.querySelectorAll=()=>[target,{focus(){advanced=true;},select(){}}];
  if(composition==='global') app.emit('compositionstart');
  app.emit('keydown',{key:'Enter',target,isComposing:composition==='event',keyCode:composition==='legacy'?229:13,preventDefault(){prevented=true;}});
  assert.equal(prevented,false); assert.equal(advanced,false);
 });
}
test('Enter advances after composition ends',()=>{
 const app=bridge(); let advanced=false,prevented=false;
 const target={dataset:{},closest(){return null;},matches(selector){return selector==='input[data-enter-next]';}};
 app.document.querySelectorAll=()=>[target,{focus(){advanced=true;},select(){}}];
 app.emit('compositionstart'); app.emit('compositionend',{target});
 app.emit('keydown',{key:'Enter',target,preventDefault(){prevented=true;}});
 assert.equal(prevented,true); assert.equal(advanced,true);
});

test('pending manual edits survive a full swap by file path',()=>{
 const app=bridge();
 const form={isConnected:true,dataset:{debounce:'600',event:'auto-save'},matches(){return true;},querySelectorAll(){return oldInputs;}};
 const oldInputs=[{dataset:{filePath:'/a'},value:'draft a'},{dataset:{filePath:'/b'},value:'draft b'}];
 const edited={...oldInputs[0],closest(){return form;}};
 app.emit('input',{target:edited});
 app.emit('htmx:before:swap',{preventDefault(){}});
 const newForm={...form};
 const newInputs=[{dataset:{filePath:'/b'},value:'server b',closest(){return newForm;}},{dataset:{filePath:'/a'},value:'server a',closest(){return newForm;}}];
 app.document.querySelectorAll=(selector)=>selector==='input[data-file-path]'?newInputs:[];
 app.emit('htmx:after:settle');
 assert.deepEqual(newInputs.map(input=>input.value),['draft b','draft a']);
});


test('selection waits for pending manual save before excluding the edited row',async()=>{
 let finishSave; const calls=[];
 const app=bridge({ajax(method,url){calls.push(url);return new Promise(resolve=>{finishSave=resolve;});}});
 const form={isConnected:true,dataset:{},values:[['name_0','draft']],matches(){return true;},getAttribute(name){return name==='hx-post'?'/api/names':name==='hx-vals'?'{}':null;}};
 app.emit('input',{target:{dataset:{},closest(){return form;}}});
 const ctx={request:{action:'/api/tick',method:'POST'},target:{id:'main-content'},fetch:async()=>{calls.push('selection');return {};}};
 app.emit('htmx:before:request',{detail:{ctx}});
 const request=ctx.fetch('/api/tick',ctx.request);
 await Promise.resolve();
 assert.equal(calls.length,1); assert.match(calls[0],/^\/api\/names\?/);
 finishSave(); await request;
 assert.equal(calls[1],'selection');
});


test('sort waits for an autosave already in flight and resolves its replacement target',async()=>{
 let finishSave; const calls=[];
 const app=bridge({ajax(){calls.push('save');return new Promise(resolve=>{finishSave=resolve;});}});
 const form={isConnected:true,dataset:{},values:[['name_0','draft']],matches(){return true;},getAttribute(name){return name==='hx-post'?'/api/names':name==='hx-vals'?'{}':null;}};
 app.emit('input',{target:{dataset:{},closest(){return form;}}});
 const save=app.window.__dubFlush();
 const replacement={id:'names-editor'};
 app.document.getElementById=()=>replacement;
 const ctx={request:{action:'/api/sort',method:'POST'},target:{id:'names-editor'},fetch:async()=>{calls.push('sort');}};
 app.emit('htmx:before:request',{detail:{ctx}});
 const request=ctx.fetch('/api/sort',ctx.request);
 await Promise.resolve(); assert.deepEqual(calls,['save']);
 finishSave(); await save; await request;
 assert.deepEqual(calls,['save','sort']); assert.equal(ctx.target,replacement);
});

test('flush serializes saves before queue-first admission and waits for the latest edit',async()=>{
 const issued=[]; let release; let active=false;
 const app=bridge({ajax(method,url){
  if(active) return Promise.resolve();
  active=true; issued.push(url);
  return new Promise(resolve=>{release=()=>{active=false;resolve();};});
 }});
 const form={isConnected:true,dataset:{},values:[['name_0','first']],matches(){return true;},getAttribute(name){return name==='hx-post'?'/api/names':name==='hx-vals'?'{}':null;}};
 const input={dataset:{},closest(){return form;}};
 app.emit('input',{target:input});
 const firstFlush=app.window.__dubFlush();
 form.values=[['name_0','second']]; app.emit('input',{target:input});
 const secondFlush=app.window.__dubFlush();
 form.values=[['name_0','latest']]; app.emit('input',{target:input});
 let finished=false;
 const latestFlush=app.window.__dubFlush().then(()=>{finished=true;});
 assert.equal(issued.length,1);
 release();
 for(let i=0;i<8;i++) await Promise.resolve();
 assert.equal(issued.length,2); assert.match(issued[1],/name_0=latest/);
 assert.equal(finished,false);
 release(); await Promise.all([firstFlush,secondFlush,latestFlush]);
 assert.equal(finished,true);
});

test('tree-only swap keeps the unchanged manual form pending',async()=>{
 const calls=[];
 const app=bridge({ajax(method,url){calls.push(url);return Promise.resolve();}});
 const input={dataset:{filePath:'/a'},value:'draft',closest(){return form;}};
 const form={isConnected:true,dataset:{},values:[['name_0','draft']],matches(){return true;},querySelectorAll(){return [input];},getAttribute(name){return name==='hx-post'?'/api/names':name==='hx-vals'?'{}':null;}};
 app.emit('input',{target:input});
 app.emit('htmx:before:swap',{detail:{ctx:{target:{id:'folder-tree'}}},preventDefault(){}});
 app.document.querySelectorAll=selector=>selector==='input[data-file-path]'?[input]:[];
 app.emit('htmx:after:settle');
 await app.window.__dubFlush();
 assert.equal(calls.length,1); assert.match(calls[0],/name_0=draft/);
});

test('manual save has a stable source independent from a waiting main-content mutation',async()=>{
 let source;
 const app=bridge({ajax(method,url,options){source=options.source;return Promise.resolve();}});
 const form={isConnected:true,dataset:{},values:[['name_0','draft']],matches(){return true;},getAttribute(name){return name==='hx-post'?'/api/names':name==='hx-vals'?'{}':null;}};
 app.emit('input',{target:{dataset:{},closest(){return form;}}});
 await app.window.__dubFlush();
 assert.equal(source,'#manual-save-source');
 const shell=shellSource;
 assert.match(shell,/<[^>]+id="manual-save-source"[^>]+hidden/);
});

test('native drop waits for pending manual save before posting paths and target',async()=>{
 const calls=[]; let finishSave;
 const app=bridge({ajax(method,url,options){
  calls.push({method,url,target:options.target});
  if(url.startsWith('/api/names?')) return new Promise(resolve=>{finishSave=resolve;});
  return Promise.resolve();
 }});
 const form={isConnected:true,dataset:{},values:[['name_0','draft']],matches(){return true;},getAttribute(name){return name==='hx-post'?'/api/names':name==='hx-vals'?'{}':null;}};
 app.emit('input',{target:{dataset:{},closest(){return form;}}});
 const drop=app.window.dubDropFiles({paths:['/a & b','/second'],target:'scan'});
 await Promise.resolve();
 assert.equal(calls.length,1); assert.match(calls[0].url,/^\/api\/names\?/);
 finishSave(); await drop;
 assert.equal(calls.length,2);
 assert.equal(calls[1].method,'POST');
 assert.equal(calls[1].url,'/api/drop?path=%2Fa+%26+b&path=%2Fsecond&target=scan');
 assert.equal(calls[1].target,'#main-content');
});
