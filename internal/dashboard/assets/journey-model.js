'use strict';

// All edits preserve the CLI configuration. Diagram coordinates are never serialized.
(function (root) {
  const copy = value => JSON.parse(JSON.stringify(value));
  const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
  const bytes = value => new TextEncoder().encode(value).length;
  const reserved = ['user_id', 'iteration', 'journey_id'];
  function shape(config) {
    if (!object(config)) throw new Error('The test configuration must be an object.');
    function stringMap(values, label) {
      if (values != null && (!object(values) || Object.values(values).some(v => typeof v !== 'string'))) throw new Error(`${label} must contain text values.`);
    }
    stringMap(config.variables, 'Test variables'); stringMap(config.headers, 'Test headers');
    if (config.scenarios === undefined || config.scenarios === null) return;
    if (!Array.isArray(config.scenarios)) throw new Error('scenarios must be an array. Correct Advanced JSON to use the canvas.');
    if (config.scenarios.length > 20) throw new Error('A test supports at most 20 journeys.');
    let count = 0;
    for (const scenario of config.scenarios) {
      if (!object(scenario) || !Array.isArray(scenario.steps)) throw new Error('Each journey needs a steps array. Correct Advanced JSON to use the canvas.');
      if (scenario.name !== undefined && typeof scenario.name !== 'string') throw new Error('Journey names must be text.');
      if (scenario.weight != null && (!Number.isInteger(scenario.weight) || scenario.weight < 1)) throw new Error('Journey weights must be positive whole numbers.');
      stringMap(scenario.variables, 'Journey variables');
      if (scenario.thresholds != null && !object(scenario.thresholds)) throw new Error('Journey thresholds must be an object.');
      if (scenario.data != null) {
        if (!Array.isArray(scenario.data)) throw new Error('Journey data must be an array.');
        for (const row of scenario.data) stringMap(row, 'Dataset rows');
      }
      if (scenario.steps.length > 50) throw new Error('A journey supports at most 50 request steps.');
      count += scenario.steps.length;
      if (count > 100) throw new Error('A test supports at most 100 request steps in total.');
      for (const step of scenario.steps) {
        if (!object(step)) throw new Error('Each journey step must be an object.');
        for (const key of ['name', 'url', 'method', 'body', 'pause']) if (step[key] !== undefined && typeof step[key] !== 'string') throw new Error(`Step ${key} must be text.`);
        for (const key of ['headers', 'extract']) if (step[key] != null && !object(step[key])) throw new Error(`Step ${key} must be an object.`);
        stringMap(step.headers, 'Step headers');
        for (const extraction of Object.values(step.extract || {})) {
          if (!object(extraction) || Object.entries(extraction).some(([k,v]) => ['json_pointer','header'].includes(k) && v != null && typeof v !== 'string')) throw new Error('Extractions must contain text selectors.');
        }
        if (step.assertions != null && (!Array.isArray(step.assertions) || step.assertions.some(a => !object(a)))) throw new Error('Step assertions must be an array of objects.');
        for (const assertion of step.assertions || []) if (Object.entries(assertion).some(([k,v]) => ['json_pointer','header','equals','body_contains'].includes(k) && v != null && typeof v !== 'string')) throw new Error('Assertion selectors and expected values must be text.');
        if (step.success_statuses != null && !Array.isArray(step.success_statuses)) throw new Error('Step success_statuses must be an array.');
      }
    }
  }
  function unique(base, names) {
    base = String(base || 'Request');
    for (let n = 1; ; n++) {
      const suffix = n === 1 ? '' : ` ${n}`;
      let prefix = base;
      while (bytes(prefix + suffix) > 128) prefix = Array.from(prefix).slice(0, -1).join('');
      const name = prefix + suffix;
      if (!names.includes(name)) return name;
    }
  }
  function stepName(step, index) { return step.name || `Step ${index + 1}`; }
  function mutate(config, action) {
    shape(config);
    const c = copy(config);
    let scenarios = c.scenarios || [], si = action.scenario ?? 0, index = action.index ?? 0;
    if (action.type === 'create') {
      if (scenarios.length) throw new Error('This test already has journeys.');
      c.scenarios = [{name: 'Journey 1', steps: [{name: 'Request 1', method: c.method || 'GET', url: c.url || '/', ...(c.body ? {body: c.body} : {})}]}];
      c.method = 'GET'; delete c.body;
      if (bytes(JSON.stringify(c)) > 1048576) throw new Error('Configuration exceeds 1 MiB.');
      return {config: c, scenario: 0, index: 0};
    }
    if (action.type === 'add-journey' || action.type === 'duplicate-journey') {
      if (scenarios.length >= 20) throw new Error('A test supports at most 20 journeys.');
      const source = action.type === 'duplicate-journey' ? scenarios[si] : {name:'Journey', steps:[{name:'Request 1',method:'GET',url:'/'}]};
      if (!source) throw new Error('Choose a journey first.');
      const scenario = copy(source);
      scenario.name = unique(action.type === 'duplicate-journey' ? source.name + ' copy' : 'Journey', scenarios.map(s => s.name));
      scenarios.push(scenario); c.scenarios = scenarios; si = scenarios.length - 1; index = 0;
    } else {
      const scenario = scenarios[si];
      if (!scenario) throw new Error('Choose a journey first.');
      const steps = scenario.steps;
      if (action.type === 'edit-journey') Object.assign(scenario, action.patch);
      else if (action.type === 'remove-journey') {
        if (scenarios.length === 1) throw new Error('Keep at least one journey.');
        scenarios.splice(si, 1); si = Math.min(si, scenarios.length - 1); index = 0;
      } else if (action.type === 'edit-test') Object.assign(c, action.patch);
      else if (action.type === 'add-step' || action.type === 'duplicate-step') {
        if (steps.length >= 50) throw new Error('A journey supports at most 50 request steps.');
        const source = action.type === 'duplicate-step' ? steps[index] : {name:'Request',method:'GET',url:'/'};
        if (!source) throw new Error('Choose a request first.');
        const step = copy(source);
        step.name = unique(action.type === 'duplicate-step' ? stepName(source, index) + ' copy' : 'Request', steps.map(stepName));
        index = Math.min(index + 1, steps.length); steps.splice(index, 0, step);
      } else if (action.type === 'remove-step') {
        if (steps.length <= 1) throw new Error('Keep at least one request in a journey.');
        if (!steps[index]) throw new Error('Choose a request first.');
        steps.splice(index, 1); index = Math.min(index, steps.length - 1);
      } else if (action.type === 'move-step') {
        if (!Number.isInteger(action.to) || action.to < 0 || action.to >= steps.length || !steps[index]) throw new Error('Choose a valid position for this request.');
        const [step] = steps.splice(index, 1); steps.splice(action.to, 0, step); index = action.to;
      } else if (action.type === 'edit-step') {
        if (!steps[index]) throw new Error('Choose a request first.');
        Object.assign(steps[index], action.patch);
      } else throw new Error('Unknown journey edit.');
    }
    if (scenarios.reduce((sum, s) => sum + s.steps.length, 0) > 100) throw new Error('A test supports at most 100 request steps in total.');
    if (scenarios.some(s => !Number.isInteger(s.weight ?? 1) || (s.weight ?? 1) < 1 || (s.weight ?? 1) > 1000)) throw new Error('Each journey weight must be a whole number between 1 and 1000.');
    if (scenarios.reduce((sum, s) => sum + (s.weight ?? 1), 0) > 10000) throw new Error('Combined journey weights must not exceed 10000.');
    if (bytes(JSON.stringify(c)) > 1048576) throw new Error('Configuration exceeds 1 MiB.');
    return {config:c, scenario:si, index};
  }
  function templates(value) {
    if (typeof value !== 'string') return [];
    return Array.from(value.matchAll(/\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:\|\s*(?:url|path|json)\s*)?\}\}/g), match => match[1]);
  }
  function dependencies(config, scenarioIndex) {
    const scenario = config.scenarios?.[scenarioIndex];
    if (!scenario) return {steps:[], edges:[]};
    const known = new Map(), edges = new Map();
    for (const [values, source] of [[config.variables,'Test variable'],[scenario.variables,'Journey variable'],[scenario.data?.[0],'Dataset row']]) {
      if (object(values)) for (const name of Object.keys(values)) known.set(name, {name,source,index:null});
    }
    for (const name of reserved) known.set(name, {name,source:'Engine value',index:null});
    const steps = scenario.steps.map((step, index) => {
      const before = Array.from(known.values()), missing = new Set(), inputs = new Map();
      function consume(text) {
        for (const name of templates(text)) {
          const origin = known.get(name);
          if (!origin) { missing.add(name); continue; }
          inputs.set(name, origin);
          if (origin.index !== null && origin.index !== index) {
            const key = `${origin.index}:${index}`;
            if (!edges.has(key)) edges.set(key, {from:origin.index,to:index,names:new Set()});
            edges.get(key).names.add(name);
          }
        }
      }
      const headers = new Map();
      for (const values of [config.headers, step.headers]) if (object(values)) for (const [name,value] of Object.entries(values)) headers.set(name.toLowerCase(),value);
      for (const value of [step.url,step.body,...headers.values()]) consume(value);
      const outputs = object(step.extract) ? Object.keys(step.extract) : [];
      // Extractions become available only after the request, before same-step assertions.
      for (const name of outputs) known.set(name, {name,source:stepName(step,index),index});
      for (const assertion of step.assertions || []) consume(assertion.equals ?? assertion.body_contains);
      return {before,inputs:Array.from(inputs.values()),outputs,missing:Array.from(missing)};
    });
    return {steps,edges:Array.from(edges.values(),edge=>({...edge,names:Array.from(edge.names)}))};
  }
  function layout(steps) {
    let y = 80;
    const nodes = steps.map((step,index)=>{const current={index,y,height:126,pause:step.pause?{y:y+150,height:30}:null};y+=step.pause?214:178;return current;});
    return {nodes,height:y+60,endY:y+8,width:560};
  }
  const model = {copy,shape,mutate,dependencies,layout,stepName,unique,templates};
  if (typeof module !== 'undefined' && module.exports) module.exports = model;
  else root.GmeterJourneyModel = model;
})(globalThis);
