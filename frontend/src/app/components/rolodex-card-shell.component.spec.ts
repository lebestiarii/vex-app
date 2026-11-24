import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CardNode } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { RolodexCardShellComponent } from './rolodex-card-shell.component';

describe('RolodexCardShellComponent', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('vex.rolodex.pinnedIds.v1', JSON.stringify([]));
  });

  afterEach(() => {
    localStorage.clear();
  });

  async function renderShell(node: CardNode<any>, depth = 0) {
    TestBed.configureTestingModule({
      imports: [RolodexCardShellComponent],
    });
    const fixture = TestBed.createComponent(RolodexCardShellComponent);
    fixture.componentRef.setInput('node', node);
    fixture.componentRef.setInput('depth', depth);
    await fixture.whenStable();
    return fixture;
  }

  it('renders the pin button ONLY on Level 0 pinnable cards (never on depth 1 or 2)', async () => {
    const baseNode: CardNode = {
      id: 'card-l0',
      template: 'kv-grid',
      icon: '📄',
      title: 'Top Card',
      chips: [{ label: 'INFO', tone: 'blue' }],
      data: { key: 'val' },
    };

    const l0 = await renderShell(baseNode, 0);
    expect(l0.nativeElement.querySelector('.pin-btn')).not.toBeNull();

    TestBed.resetTestingModule();
    const l0Unpinnable = await renderShell({ ...baseNode, pinnable: false }, 0);
    expect(l0Unpinnable.nativeElement.querySelector('.pin-btn')).toBeNull();

    TestBed.resetTestingModule();
    const l1 = await renderShell(baseNode, 1);
    expect(l1.nativeElement.querySelector('.pin-btn')).toBeNull();

    TestBed.resetTestingModule();
    const l2 = await renderShell(baseNode, 2);
    expect(l2.nativeElement.querySelector('.pin-btn')).toBeNull();
  });

  it('renders WebsiteLink Single-Button Mode (1 link) without chevron and opens URL on click', async () => {
    const node: CardNode = {
      id: 'single-link-1',
      template: 'website-link',
      icon: '🌐',
      title: 'CBP ACE Portal',
      chips: [{ label: 'PROD', tone: 'red', dot: 'red' }],
      data: { url: 'https://ace.cbp.dhs.gov' },
    };

    const fixture = await renderShell(node, 1);
    const state = TestBed.inject(RolodexStateService);
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});

    const btn = fixture.nativeElement.querySelector('button.single-link-card') as HTMLButtonElement | null;
    expect(btn).not.toBeNull();
    expect(fixture.nativeElement.querySelector('.chevron')).toBeNull();
    expect(fixture.nativeElement.querySelector('.external-arrow')?.textContent?.trim()).toBe('↗');

    btn!.click();
    expect(openSpy).toHaveBeenCalledWith('https://ace.cbp.dhs.gov');
  });

  it('renders WebsiteLink Multi-Chip Mode (2+ links) as an expandable card with chevron', async () => {
    const node: CardNode = {
      id: 'multi-link-1',
      template: 'website-link',
      icon: '🔗',
      title: 'Port Portals',
      chips: [
        { label: 'QA', tone: 'amber', dot: 'amber' },
        { label: 'PROD', tone: 'red', dot: 'red' },
      ],
      data: {
        bookmarks: [
          {
            name: 'Customs Portal',
            links: [
              { label: 'QA', tone: 'amber', url: 'https://qa.example.test' },
              { label: 'PROD', tone: 'red', url: 'https://prod.example.test' },
            ],
          },
        ],
      },
    };

    const fixture = await renderShell(node, 0);
    expect(fixture.nativeElement.querySelector('button.single-link-card')).toBeNull();
    expect(fixture.nativeElement.querySelector('.chevron')).not.toBeNull();

    const header = fixture.nativeElement.querySelector('.header-strip') as HTMLElement;
    header.click();
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('app-website-link-card')).not.toBeNull();
  });

  it('keeps location cards strictly header-only and caps recursive nesting at depth 2', async () => {
    const deepTree: CardNode = {
      id: 'loc-ams',
      template: 'location',
      icon: '🇳🇱',
      title: 'Amsterdam Gateway (AMS)',
      defaultExpanded: true,
      chips: [{ label: 'NLAMS · AMS', tone: 'blue' }],
      data: { ianaTimezone: 'Europe/Amsterdam' },
      children: [
        {
          id: 'child-l1',
          template: 'location',
          icon: '📦',
          title: 'Level 1 Group',
          defaultExpanded: true,
          chips: [],
          data: {},
          children: [
            {
              id: 'child-l2',
              template: 'kv-grid',
              icon: '🔹',
              title: 'Level 2 Leaf',
              defaultExpanded: true,
              chips: [],
              data: { host: 'leaf.internal' },
              children: [
                {
                  id: 'child-l3-ignored',
                  template: 'kv-grid',
                  icon: '🚫',
                  title: 'Level 3 Should Not Render',
                  defaultExpanded: true,
                  chips: [],
                  data: {},
                },
              ],
            },
          ],
        },
      ],
    };

    const fixture = await renderShell(deepTree, 0);
    const rootArticle = fixture.nativeElement.querySelector('article[data-depth="0"]') as HTMLElement;
    expect(rootArticle).not.toBeNull();

    // Location L0 card must NOT render a direct .leaf-slot body
    const directLeafSlot = rootArticle.querySelector(':scope > .expanded-body > .leaf-slot');
    expect(directLeafSlot).toBeNull();

    // Depth 1 and Depth 2 render inside full-width .nested-stack; Depth 3 is capped out
    expect(fixture.nativeElement.querySelector('[data-depth="1"]')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('[data-depth="2"]')).not.toBeNull();
    expect(fixture.nativeElement.textContent).toContain('Level 2 Leaf');
    expect(fixture.nativeElement.textContent).not.toContain('Level 3 Should Not Render');
  });

  it('syncs isExpanded via linkedSignal when node input changes while allowing local toggles', async () => {
    const collapsedNode: CardNode = {
      id: 'c1',
      template: 'kv-grid',
      icon: '📄',
      title: 'Card One',
      defaultExpanded: false,
      chips: [],
      data: { a: '1' },
    };

    const fixture = await renderShell(collapsedNode, 0);
    expect(fixture.componentInstance.isExpanded()).toBe(false);

    fixture.componentInstance.toggleExpanded();
    await fixture.whenStable();
    expect(fixture.componentInstance.isExpanded()).toBe(true);

    // Passing a new node input with defaultExpanded: false resets linkedSignal cleanly
    fixture.componentRef.setInput('node', { ...collapsedNode, id: 'c2', defaultExpanded: false });
    await fixture.whenStable();
    expect(fixture.componentInstance.isExpanded()).toBe(false);

    fixture.componentRef.setInput('node', { ...collapsedNode, id: 'c3', defaultExpanded: true });
    await fixture.whenStable();
    expect(fixture.componentInstance.isExpanded()).toBe(true);
  });
});
