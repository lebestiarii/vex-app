import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CardNode,
  ConversionData,
  PatternBreakdownData,
  ServerMatrixData,
  WebsiteLinkData,
} from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { ConversionCardComponent } from './conversion-card.component';
import { PatternBreakdownCardComponent } from './pattern-breakdown-card.component';
import { ServerMatrixCardComponent } from './server-matrix-card.component';
import { WebsiteLinkCardComponent } from './website-link-card.component';

describe('Canonical Card Templates', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('ServerMatrixCardComponent renders stoplight rows and triggers Copy, SSH, and Open', async () => {
    TestBed.configureTestingModule({ imports: [ServerMatrixCardComponent] });
    const fixture = TestBed.createComponent(ServerMatrixCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});

    const node: CardNode<ServerMatrixData> = {
      id: 'srv-1',
      template: 'server-matrix',
      icon: '🖥️',
      title: 'EDI Servers',
      chips: [],
      data: {
        rows: [
          {
            badge: 'DEV',
            tone: 'green',
            host: 'edi-dev.internal',
            port: '2222',
            sshCommand: 'ssh ops@edi-dev.internal -p 2222',
            url: 'https://edi-dev.internal',
          },
          { badge: 'PROD', tone: 'red', host: 'edi-prd.internal' },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const rows = fixture.nativeElement.querySelectorAll('.matrix-row');
    expect(rows.length).toBe(2);
    expect(rows[0].querySelector('.host-name')?.textContent?.trim()).toBe('edi-dev.internal:2222');

    const buttons = rows[0].querySelectorAll('button.action-btn') as NodeListOf<HTMLButtonElement>;
    buttons[0].click(); // Copy
    buttons[1].click(); // SSH
    buttons[2].click(); // Open

    expect(copySpy).toHaveBeenCalledWith('DEV Host', 'edi-dev.internal');
    expect(copySpy).toHaveBeenCalledWith('DEV SSH', 'ssh ops@edi-dev.internal -p 2222');
    expect(openSpy).toHaveBeenCalledWith('https://edi-dev.internal');
  });

  it('WebsiteLinkCardComponent renders multi-environment links and Copy button', async () => {
    TestBed.configureTestingModule({ imports: [WebsiteLinkCardComponent] });
    const fixture = TestBed.createComponent(WebsiteLinkCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();
    const openSpy = vi.spyOn(state, 'openExternalUrl').mockImplementation(() => {});

    const node: CardNode<WebsiteLinkData> = {
      id: 'wl-1',
      template: 'website-link',
      icon: '🔗',
      title: 'Customs Links',
      chips: [],
      data: {
        bookmarks: [
          {
            name: 'DMS Portal',
            subtitle: 'Declarations',
            links: [
              { label: 'QA', tone: 'amber', url: 'https://dms-qa.example.test' },
              { label: 'PROD', tone: 'red', url: 'https://dms.example.test' },
            ],
          },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const envBtns = fixture.nativeElement.querySelectorAll('button.env-launch') as NodeListOf<HTMLButtonElement>;
    expect(envBtns.length).toBe(2);
    envBtns[0].click();
    expect(openSpy).toHaveBeenCalledWith('https://dms-qa.example.test');

    const copyBtn = fixture.nativeElement.querySelector('button.copy-url-btn') as HTMLButtonElement;
    copyBtn.click();
    expect(copySpy).toHaveBeenCalledWith('DMS Portal URL', 'https://dms.example.test');
  });

  it('PatternBreakdownCardComponent renders 3-column Blue/Amber/Green tiles and explanation', async () => {
    TestBed.configureTestingModule({ imports: [PatternBreakdownCardComponent] });
    const fixture = TestBed.createComponent(PatternBreakdownCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<PatternBreakdownData> = {
      id: 'pat-1',
      template: 'pattern-breakdown',
      icon: '🧩',
      title: 'House Bill · 7031234567',
      chips: [],
      data: {
        segments: [
          { value: '7', role: 'MODE', meaning: 'Ocean Import', tone: 'blue' },
          { value: '03', role: 'BRANCH', meaning: 'AMS', tone: 'amber' },
          { value: '1234567', role: 'SEQUENCE', meaning: 'Shipment #', tone: 'green' },
        ],
        explanation: 'Forwarder-issued House Bill of Lading.',
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    const tiles = fixture.nativeElement.querySelectorAll('button.segment-tile') as NodeListOf<HTMLButtonElement>;
    expect(tiles.length).toBe(3);
    expect(tiles[0].getAttribute('data-tone')).toBe('blue');
    expect(tiles[1].getAttribute('data-tone')).toBe('amber');
    expect(tiles[2].getAttribute('data-tone')).toBe('green');
    expect(fixture.nativeElement.querySelector('.breakdown-explanation')?.textContent?.trim()).toBe(
      'Forwarder-issued House Bill of Lading.'
    );

    tiles[1].click();
    expect(copySpy).toHaveBeenCalledWith('BRANCH', '03');
  });

  it('ConversionCardComponent renders Input ⇄ Output strip and secondary equivalents', async () => {
    TestBed.configureTestingModule({ imports: [ConversionCardComponent] });
    const fixture = TestBed.createComponent(ConversionCardComponent);
    const state = TestBed.inject(RolodexStateService);
    const copySpy = vi.spyOn(state, 'copyText').mockResolvedValue();

    const node: CardNode<ConversionData> = {
      id: 'conv-1',
      template: 'conversion',
      icon: '⚖️',
      title: 'Weight Conversion',
      chips: [],
      data: {
        input: '100 kg',
        output: '220.46 lb',
        equivalents: [
          { label: 'Metric Ton', value: '0.10 t' },
          { label: 'Ounces', value: '3,527.40 oz' },
        ],
      },
    };
    fixture.componentRef.setInput('node', node);
    await fixture.whenStable();

    expect(fixture.nativeElement.querySelector('.conv-input')?.textContent?.trim()).toBe('100 kg');
    expect(fixture.nativeElement.querySelector('.conv-output')?.textContent?.trim()).toBe('220.46 lb');

    const copyBtn = fixture.nativeElement.querySelector('button.copy-btn') as HTMLButtonElement;
    copyBtn.click();
    expect(copySpy).toHaveBeenCalledWith('Converted Value', '220.46 lb');

    const equivBtns = fixture.nativeElement.querySelectorAll('button.equiv-chip') as NodeListOf<HTMLButtonElement>;
    expect(equivBtns.length).toBe(2);
    equivBtns[0].click();
    expect(copySpy).toHaveBeenCalledWith('Metric Ton', '0.10 t');
  });
});
