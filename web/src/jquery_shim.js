import jQuery from 'jquery';

jQuery.prototype.trigger = function(type) {
  for (let el of this) {
    el.dispatchEvent(new Event(type));
  }
};
